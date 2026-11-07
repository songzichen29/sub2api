package qqcommunity

import (
	"context"
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func lotteryRechargeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"pay_amount", "fee_rate", "amount", "refund_amount", "status", "provider_key", "payment_type", "provider_snapshot"})
}

func expectLotteryRecharge(m sqlmock.Sqlmock, uid int64, principal string) {
	m.ExpectQuery(`SELECT pay_amount,fee_rate,amount,refund_amount,status,`).WithArgs(uid, lotteryPeriodStart).
		WillReturnRows(lotteryRechargeRows().AddRow(principal, "0", principal, "0", "COMPLETED", "alipay", "alipay", nil))
}

func TestLotteryOrderPrincipalUsesActualPayment(t *testing.T) {
	for _, tc := range []struct {
		name, paid, fee, credited, refunded, status, want string
	}{
		{"30 RMB credited as 60 USD", "30.18", "0.6", "60", "0", "COMPLETED", "30"},
		{"different multiplier and bonus", "30.18", "0.6", "150", "0", "COMPLETED", "30"},
		{"coupon and discount reduce actual payment", "20.12", "0.6", "60", "0", "COMPLETED", "20"},
		{"rounded up fee", "10.02", "0.6", "19.9", "0", "COMPLETED", "9.96"},
		{"tiny recharge", "0.02", "0.6", "0.02", "0", "COMPLETED", "0.01"},
		{"no fee", "30", "0", "60", "0", "COMPLETED", "30"},
		{"failed refund restored", "30.18", "0.6", "60", "30", "REFUND_FAILED", "30"},
		{"partial refund", "30.18", "0.6", "60", "20", "PARTIALLY_REFUNDED", "20"},
		{"fractional partial refund", "1.01", "0.6", "3", "1", "PARTIALLY_REFUNDED", "0.66666667"},
		{"full refund", "30.18", "0.6", "60", "60", "REFUNDED", "0"},
		{"refund exceeds amount", "30.18", "0.6", "60", "61", "PARTIALLY_REFUNDED", "0"},
		{"pending refund", "30.18", "0.6", "60", "0", "REFUND_PENDING", "0"},
		{"refunding", "30.18", "0.6", "60", "0", "REFUNDING", "0"},
		{"payment not credited", "30.18", "0.6", "60", "0", "PAID", "0"},
		{"not paid", "0", "0.6", "60", "0", "COMPLETED", "0"},
		{"invalid fee", "30", "-1", "60", "0", "COMPLETED", "0"},
		{"invalid cents", "30.181", "0.6", "60", "0", "COMPLETED", "0"},
		{"inconsistent stored fee", "0.01", "0.6", "0.02", "0", "COMPLETED", "0"},
		{"invalid refunded amount", "30.18", "0.6", "60", "-1", "PARTIALLY_REFUNDED", "0"},
		{"zero refund denominator", "30.18", "0.6", "0", "1", "PARTIALLY_REFUNDED", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := lotteryOrderPrincipal(decimal.RequireFromString(tc.paid), decimal.RequireFromString(tc.fee), decimal.RequireFromString(tc.credited), decimal.RequireFromString(tc.refunded), tc.status)
			require.True(t, got.Equal(decimal.RequireFromString(tc.want)), "got %s, want %s", got, tc.want)
		})
	}
}

func TestLotteryPrincipalExactlyReversesRoundedFees(t *testing.T) {
	for _, fee := range []string{"0", "0.0001", "0.6", "2.99", "100"} {
		rate := decimal.RequireFromString(fee)
		for cents := int64(1); cents <= 10000; cents++ {
			principal := decimal.New(cents, -2)
			paid := principal.Add(principal.Mul(rate).Div(decimal.NewFromInt(100)).RoundUp(2))
			got := lotteryOrderPrincipal(paid, rate, principal.Mul(decimal.NewFromInt(2)), decimal.Zero, "COMPLETED")
			require.True(t, principal.Equal(got), "principal=%s, fee=%s, paid=%s, got=%s", principal, fee, paid, got)
		}
	}
}

func TestLotteryPaymentCurrency(t *testing.T) {
	for _, tc := range []struct {
		name, provider, method, snapshot string
		want                             bool
	}{
		{"legacy native alipay", "alipay", "alipay", `{"provider_key":"alipay"}`, true},
		{"legacy wxpay", "wxpay", "wxpay", "", true},
		{"legacy easypay", "easypay", "alipay", "", true},
		{"snapshot provider", "", "", `{"provider_key":"alipay"}`, true},
		{"legacy method", "", "alipay", "", true},
		{"explicit CNY", "stripe", "card", `{"currency":"cny"}`, true},
		{"explicit USD", "alipay", "alipay", `{"currency":"USD"}`, false},
		{"explicit other currency", "stripe", "alipay", `{"currency":"JPY"}`, false},
		{"unknown provider with CNY method", "stripe", "alipay", "", false},
		{"snapshot unknown provider", "", "alipay", `{"provider_key":"stripe"}`, false},
		{"unknown legacy", "", "card", "", false},
		{"invalid snapshot", "alipay", "alipay", `{`, false},
		{"invalid currency", "alipay", "alipay", `{"currency":123}`, false},
		{"empty currency", "alipay", "alipay", `{"currency":""}`, false},
		{"null currency", "alipay", "alipay", `{"currency":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, lotteryPaymentIsCNY(tc.provider, tc.method, []byte(tc.snapshot)))
		})
	}
}

func TestLotteryRechargeQueryBothDialects(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		for _, lock := range []bool{false, true} {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			s := New(db, nil, false, nil)
			s.db.mysql = mysql
			q := `SELECT pay_amount,fee_rate,amount,refund_amount,status,COALESCE(provider_key,''),payment_type,provider_snapshot
        FROM payment_orders WHERE user_id=$1 AND order_type='balance'
        AND paid_at IS NOT NULL AND completed_at IS NOT NULL AND completed_at >= $2 ORDER BY id`
			if lock {
				q += ` FOR SHARE`
			}
			bound, args, err := bindSQL(mysql, q, []any{int64(42), lotteryPeriodStart})
			require.NoError(t, err)
			m.ExpectQuery(regexp.QuoteMeta(bound)+"$").WithArgs(args[0], args[1]).WillReturnRows(lotteryRechargeRows().
				AddRow("5.03", "0.6", "10", "0", "COMPLETED", "alipay", "alipay", nil).
				AddRow("5.03", "0.6", "100", "0", "COMPLETED", "alipay", "alipay", nil).
				AddRow("30.18", "0.6", "60", "0", "COMPLETED", "stripe", "card", `{"currency":"USD"}`))
			total, err := lotteryRechargeSum(context.Background(), s.db, 42, lock)
			require.NoError(t, err)
			require.Equal(t, "10", total.String())
			require.NoError(t, m.ExpectationsWereMet(), "balance codes must not be queried")
			m.ExpectClose()
			require.NoError(t, db.Close())
		}
	}
}

func TestLotteryStateCorrectsEntitlementAndPreservesUsageAndGrants(t *testing.T) {
	for _, tc := range []struct {
		paid                            string
		used, grants, earned, remaining int64
		next                            float64
	}{
		{"30.18", 0, 0, 3, 3, 10},
		{"30.18", 1, 2, 5, 4, 10},
		{"30.18", 6, 0, 3, 0, 40},
		{"0", 1, 0, 0, 0, 20},
	} {
		db, m, err := sqlmock.New()
		require.NoError(t, err)
		m.ExpectQuery(`SELECT pay_amount,fee_rate,amount,refund_amount,status,`).WithArgs(int64(42), lotteryPeriodStart).
			WillReturnRows(lotteryRechargeRows().AddRow(tc.paid, "0.6", "60", "0", "COMPLETED", "alipay", "alipay", nil))
		m.ExpectQuery(`SELECT COUNT\(\*\) FROM qqc_lottery_draws`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.used))
		m.ExpectQuery(`SELECT COALESCE\(SUM\(chances\)`).WithArgs(int64(42), lotteryPeriodStart).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(tc.grants))
		state, err := lotteryState(context.Background(), New(db, nil, false, nil).db, defaultLotteryConfig(), 42, false)
		require.NoError(t, err)
		require.Equal(t, tc.earned, state.Earned)
		require.Equal(t, tc.remaining, state.Remaining)
		require.Equal(t, tc.used, state.Used)
		require.Equal(t, tc.grants, state.BonusChances)
		require.Equal(t, tc.next, state.NextRecharge)
		require.NoError(t, m.ExpectationsWereMet(), "recalculation must not mutate draws, balances or grants")
		m.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestLotteryRechargeDoesNotReturnPartialTotalOnRowError(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	m.ExpectQuery(`SELECT pay_amount,fee_rate,amount,refund_amount,status,`).WillReturnRows(lotteryRechargeRows().AddRow("30.18", "0.6", "60", "0", "COMPLETED", "alipay", "alipay", nil).RowError(0, errors.New("read failed")))
	_, err = lotteryRechargeSum(context.Background(), New(db, nil, false, nil).db, 42, false)
	require.ErrorContains(t, err, "read failed")
	require.NoError(t, m.ExpectationsWereMet())
}
