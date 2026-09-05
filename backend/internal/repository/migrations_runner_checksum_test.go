package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsMigrationChecksumCompatible(t *testing.T) {
	t.Run("001 MySQL baseline历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_baseline.sql",
			"6e6be968868f5f71045f1ae98809994e1bb3b5dbc35aa37bc1d7a159492775fd",
			"e5f2aca8139d8639798bab6e01917f0d85794e4307e48692a0a2f63ec6a24071",
		)
		require.True(t, ok)
	})

	t.Run("001 MySQL baseline未知checksum不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_baseline.sql",
			"6e6be968868f5f71045f1ae98809994e1bb3b5dbc35aa37bc1d7a159492775fd",
			"0000000000000000000000000000000000000000000000000000000000000000",
		)
		require.False(t, ok)
	})

	t.Run("MySQL early baseline历史checksum可兼容", func(t *testing.T) {
		cases := []struct {
			name string
			db   string
			file string
		}{
			{"002_auth_profile_followup.sql", "8809d39ce4f322350ea17fe9e92382af3713e0988232f358afde0eaa10764c27", "d7a4fd85315d20f99d168e8e205a249ae8e360a67719d8bb0527ad44d119dee5"},
			{"003_runtime_schema_parity.sql", "a1357af4ee207fd7786585b21c19e2a671ea4a776c25f307346184d88c7fe81a", "f8b805bfdf2269a8e67a905b8db4bd7a26cf94af01b816c15de5770264627451"},
			{"004_add_user_subscription_source.sql", "99941c83582447cb7546345759b4207a90061fcbb77772f3c48ea6eedeac206d", "a7ab8da86d7de0def3847ec8ca415db0b32d57ee9739a9fd4071a12a33b626db"},
			{"005_add_account_tags.sql", "8528cd9f411d797506545b66169ded42e2da207c2e110647d86ba717c8b81009", "490962553e69756a500bad22e2ba1cf38650ec764bfa23cd44d42d90fccb2cf3"},
			{"006_affiliate_custom_settings.sql", "cf719754e77d206f9145725fbcb843650486bf0dee4b49b06ed50da598fccfc6", "449d10b5241f19e1e3257eaac0a98d9ef7b48690a95fc17204b8c3dee8b444d6"},
			{"007_affiliate_custom_kind_rates.sql", "52d3e049281e7d1eef0b75434d801b0fc77066aeee80de70cc237ea7b2670642", "90baca551732d592199a8cc7b9e48df2d0d5d1f0e3b6ebbb7835213dcb7e904c"},
			{"007_affiliate_rebate_freeze.sql", "91e4750b3af5c6f4aa30ab9858b90dd25c30a1341cd376c7a7ee2267f54d293e", "ab685e380a2958abeae44b95ec4d8499d4d554041d8a5f6d0b121b5b4cf1c93e"},
			{"008_affiliate_ledger_audit_snapshots.sql", "322f59fa3f74206783996538ae6f1c811263ab57936413aebe0f76d0b26620bd", "5b1fcbef2113a709e25789404831485431963652db3db920da67335c357f2d3f"},
			{"009_image_generation_group_controls.sql", "005a6e56de37e030a5228d00d805f1626beb7d4aea56461a81ea59002bdd1bc0", "00d659f4fa3a367f8dbd4858344ee1b5912254460f1e66ae6568e9714d568202"},
			{"010_daily_limit_reset_payment.sql", "c1cca2f9d3be263e8014c9fa76882b7eab729534f5ea9ecdd498c2adbe3ff5b6", "23428614a6195599230a93bf4bed6a73888a868b6e4d4f013d15d76321cab933"},
			{"013_add_user_subscription_validity_unit.sql", "768b7d6b71bb6b3bd58b2f4216c00b320c0fe59c7d95a7af6c84b93422833105", "37c42ec7071234ef0366e8a87f734dc60545995249f5b7da61bfcdf26342053a"},
			{"014_add_payment_order_subscription_validity_unit.sql", "3b24cc5074ed8900a2a58084ba116374f07d348125e3f82c77999763d06b2446", "dbac228db8d83f33456bffd78f645de3f9202d650c6442b3b0993c1c58484a22"},
		}
		for _, tc := range cases {
			require.True(t, isMigrationChecksumCompatible(tc.name, tc.db, tc.file), tc.name)
			require.False(t, isMigrationChecksumCompatible(tc.name, tc.db, "0000000000000000000000000000000000000000000000000000000000000000"), tc.name)
		}
	})

	t.Run("015 usage log image metadata历史checksum可兼容", func(t *testing.T) {
		for _, fileChecksum := range []string{
			"5e332af35c5bfcd9ec285536ad90029a0d30a09820b7b7baadf2a798169d44d0",
			"dc56bf1b8e89710fd262de0110edb366498b7c6f953f70c00f5a1ddbdc676d3c",
			"ee099bd31e8a60eb4f45a47443a1b6d47e7ca3ed9c1988e135fd5742adb629a0",
		} {
			ok := isMigrationChecksumCompatible(
				"015_usage_log_image_size_metadata.sql",
				"257f5b52afce019b2738b8ecc8936d558e4cb9b1369898ee462661d904a540ce",
				fileChecksum,
			)
			require.True(t, ok, fileChecksum)
		}
	})

	t.Run("054历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.True(t, ok)
	})

	t.Run("054在未知文件checksum下不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"0000000000000000000000000000000000000000000000000000000000000000",
		)
		require.False(t, ok)
	})

	t.Run("061历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("061第二个历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("076历史checksum可兼容修复后的MySQL迁移", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"076_channel_monitor_v2.sql",
			"8b9b37935c1e378ca4a8f7461487d391c2995bcec0898a70e5601932c0e5e93e",
			"988d13f8dfea063b4246f6b24027f24de329b819cd90ece29be33deb01515678",
		)
		require.True(t, ok)
	})

	t.Run("非白名单迁移不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_init.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.False(t, ok)
	})

	t.Run("109历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109当前checksum可兼容历史checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109回滚到历史文件后仍兼容已应用的新checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		)
		require.True(t, ok)
	})

	t.Run("110历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"110_pending_auth_and_provider_default_grants.sql",
			"e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925",
			"32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279",
		)
		require.True(t, ok)
	})

	t.Run("112历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"112_add_payment_order_provider_key_snapshot.sql",
			"ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e",
			"b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99",
		)
		require.True(t, ok)
	})

	t.Run("115历史checksum可兼容修复后的legacy external backfill", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"115_auth_identity_legacy_external_backfill.sql",
			"4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f",
			"022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f",
		)
		require.True(t, ok)
	})

	t.Run("116历史checksum可兼容修复后的legacy external safety reports", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"116_auth_identity_legacy_external_safety_reports.sql",
			"f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877",
			"07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488",
		)
		require.True(t, ok)
	})

	t.Run("119历史checksum可兼容占位文件", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e",
		)
		require.True(t, ok)
	})

	t.Run("118多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb",
			"e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227",
		} {
			ok := isMigrationChecksumCompatible(
				"118_wechat_dual_mode_and_auth_source_defaults.sql",
				dbChecksum,
				"b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0",
			)
			require.True(t, ok)
		}
	})

	t.Run("120多个历史checksum都可兼容新的notx修复版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61",
			"707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22",
			"04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a",
		} {
			ok := isMigrationChecksumCompatible(
				"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
				dbChecksum,
				"34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074",
			)
			require.True(t, ok)
		}
	})

	t.Run("119未知checksum不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		)
		require.False(t, ok)
	})
}
