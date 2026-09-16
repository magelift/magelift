// Command gcp-cloudsql-destroy-retention-acceptance proves leftover Cloud SQL
// backups remain after instance deletion, then deletes only those backups
// through the MageLift translator. The shell wrapper owns instance create and
// delete; this command must not create or delete instances.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
)

const defaultTimeout = 10 * time.Minute

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "gcp Cloud SQL destroy-retention acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("gcp-cloudsql-destroy-retention-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "GCP project ID")
	instance := flags.String("instance", "", "destroyed Magelift Cloud SQL instance ID")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := validateName(*project, "project"); err != nil {
		return err
	}
	if err := validateName(*instance, "instance"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	api, err := gcpresilience.NewGCPCloudSQLNativeAPI(ctx, gcpresilience.NativeAPIConfig{Project: *project})
	if err != nil {
		return err
	}
	leftovers, err := api.LeftoverCloudSQLBackupsForInstance(ctx, *instance)
	if err != nil {
		return err
	}
	if len(leftovers) == 0 {
		return errors.New("no leftover Cloud SQL backups remained after instance deletion")
	}
	types := uniqueBackupTypes(leftovers)
	deleted, err := api.DeleteLeftoverCloudSQLBackupsForInstance(ctx, *instance)
	if err != nil {
		return err
	}
	remaining, err := api.LeftoverCloudSQLBackupsForInstance(ctx, *instance)
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return fmt.Errorf("leftover Cloud SQL backups remain after MageLift cleanup: %d", len(remaining))
	}
	fmt.Fprintf(output, "gcp Cloud SQL destroy-retention PASS project=%s\n", *project)
	fmt.Fprintf(output, "instance=%s leftoverCount=%d leftoverTypes=%s deletedCount=%d remaining=0\n", *instance, len(leftovers), strings.Join(types, ","), len(deleted))
	return nil
}

func uniqueBackupTypes(backups []gcpresilience.CloudSQLBackup) []string {
	seen := make(map[string]struct{}, len(backups))
	types := make([]string, 0, len(backups))
	for _, backup := range backups {
		kind := strings.TrimSpace(backup.Type)
		if kind == "" {
			kind = "unspecified"
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		types = append(types, kind)
	}
	sort.Strings(types)
	return types
}

func validateName(value, name string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\x00/*?") {
		return fmt.Errorf("%s must be a single Cloud SQL name", name)
	}
	return nil
}
