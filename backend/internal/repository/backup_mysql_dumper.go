package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// MySQLDumper implements service.DBDumper using mysqldump/mysql
type MySQLDumper struct {
	cfg            *config.DatabaseConfig
	db             *sql.DB
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewMySQLDumper creates a new MySQLDumper
func NewMySQLDumper(cfg *config.Config, db *sql.DB) service.DBDumper {
	return &MySQLDumper{
		cfg:            &cfg.Database,
		db:             db,
		commandContext: exec.CommandContext,
	}
}

// Dump executes mysqldump and returns a streaming reader of the output
func (d *MySQLDumper) Dump(ctx context.Context) (io.ReadCloser, error) {
	if d.db == nil {
		return nil, errors.New("acquire backup migration lock: nil sql db")
	}
	lockConn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire backup migration lock connection: %w", err)
	}
	if err := acquireMigrationsLock(ctx, lockConn); err != nil {
		discardBackupSQLConnection(lockConn)
		return nil, fmt.Errorf("acquire backup migration lock: %w", err)
	}
	releaseLock := func() error { return releaseBackupMigrationLock(lockConn) }

	args := []string{
		"-h", d.cfg.Host,
		"-P", fmt.Sprintf("%d", d.cfg.Port),
		"-u", d.cfg.User,
		fmt.Sprintf("-p%s", d.cfg.Password),
		"--single-transaction",
		"--routines",
		"--triggers",
		"--set-gtid-purged=OFF",
		d.cfg.DBName,
	}

	commandContext := d.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(ctx, "mysqldump", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create stdout pipe: %w", err), releaseLock())
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, errors.Join(fmt.Errorf("start mysqldump: %w", err), releaseLock())
	}

	return &cmdReadCloser{ReadCloser: stdout, cmd: cmd, release: releaseLock}, nil
}

func releaseBackupMigrationLock(conn *sql.Conn) error {
	unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := releaseMigrationsLock(unlockCtx, conn); err != nil {
		discardBackupSQLConnection(conn)
		return fmt.Errorf("release backup migration lock: %w", err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close backup migration lock connection: %w", err)
	}
	return nil
}

func discardBackupSQLConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// Restore executes mysql to restore from a streaming reader
func (d *MySQLDumper) Restore(ctx context.Context, data io.Reader) error {
	args := []string{
		"-h", d.cfg.Host,
		"-P", fmt.Sprintf("%d", d.cfg.Port),
		"-u", d.cfg.User,
		fmt.Sprintf("-p%s", d.cfg.Password),
		d.cfg.DBName,
	}

	cmd := exec.CommandContext(ctx, "mysql", args...)
	cmd.Stdin = data

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

// cmdReadCloser wraps a command stdout pipe and waits for the process on Close
type cmdReadCloser struct {
	io.ReadCloser
	cmd       *exec.Cmd
	release   func() error
	closeOnce sync.Once
	closeErr  error
}

func (c *cmdReadCloser) Close() error {
	c.closeOnce.Do(func() {
		var closeErrs []error
		if err := c.ReadCloser.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("close mysqldump stdout: %w", err))
		}
		if err := c.cmd.Wait(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("mysqldump exited with error: %w", err))
		}
		if c.release != nil {
			if err := c.release(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}
		c.closeErr = errors.Join(closeErrs...)
	})
	return c.closeErr
}
