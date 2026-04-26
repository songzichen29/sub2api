package repository

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// MySQLDumper implements service.DBDumper using mysqldump/mysql
type MySQLDumper struct {
	cfg *config.DatabaseConfig
}

// NewMySQLDumper creates a new MySQLDumper
func NewMySQLDumper(cfg *config.Config) service.DBDumper {
	return &MySQLDumper{cfg: &cfg.Database}
}

// Dump executes mysqldump and returns a streaming reader of the output
func (d *MySQLDumper) Dump(ctx context.Context) (io.ReadCloser, error) {
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

	cmd := exec.CommandContext(ctx, "mysqldump", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mysqldump: %w", err)
	}

	return &cmdReadCloser{ReadCloser: stdout, cmd: cmd}, nil
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
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	_ = c.ReadCloser.Close()
	if err := c.cmd.Wait(); err != nil {
		return fmt.Errorf("mysqldump exited with error: %w", err)
	}
	return nil
}
