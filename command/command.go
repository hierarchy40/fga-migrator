// Package command implements ready-made migrate and schemaversion mains.
package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	fgamigrator "github.com/hierarchy40/fga-migrator"
	"github.com/kelseyhightower/envconfig"
)

type migrateEnv struct {
	fgamigrator.Config
	WaitTimeout time.Duration `envconfig:"FGA_WAIT_TIMEOUT" default:"5m"`
}

// Migrate reads the FGA_* variables and FGA_WAIT_TIMEOUT, then runs fgamigrator.Run.
func Migrate(migrations *fgamigrator.Migrations) {
	if err := runMigrate(context.Background(), migrations); err != nil {
		log.Fatal(err)
	}
	log.Println("all migrations applied successfully")
}

// SchemaVersion prints the required schema version, or with -model-digest the model digest.
func SchemaVersion(migrations *fgamigrator.Migrations) {
	if err := runSchemaVersion(os.Args[1:], os.Stdout, migrations); err != nil {
		log.Fatal(err)
	}
}

func runMigrate(ctx context.Context, migrations *fgamigrator.Migrations) error {
	var env migrateEnv
	if err := envconfig.Process("", &env); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return fgamigrator.Run(ctx, env.Config, migrations, env.WaitTimeout)
}

func runSchemaVersion(args []string, out io.Writer, migrations *fgamigrator.Migrations) error {
	flags := flag.NewFlagSet("schemaversion", flag.ContinueOnError)
	modelDigest := flags.Bool("model-digest", false, "print sha256:<hex> of the normalised model instead")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if !*modelDigest {
		required, err := migrations.Required()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, required)
		return err
	}
	digest, err := migrations.ModelDigest()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, digest)
	return err
}
