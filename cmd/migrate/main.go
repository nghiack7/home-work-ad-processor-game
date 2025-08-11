package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/personal/home-work-ad-process/pkg/config"
	"github.com/pressly/goose/v3"
)

// Build information set via ldflags
var (
	Version     = "dev"
	BuildCommit = "local"
	BuildTime   = "unknown"
	GoVersion   = "unknown"
)

func main() {
	var (
		command     = flag.String("cmd", "", "Command to run: up, down, down-to, redo, reset, status, version, create")
		target      = flag.String("target", "", "Target version for down-to command")
		name        = flag.String("name", "", "Name for create command")
		migType     = flag.String("type", "sql", "Migration type: sql or go")
		verbose     = flag.Bool("v", false, "Verbose output")
		showVersion = flag.Bool("version", false, "Show version information")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Ad Processing Migration Tool (Goose-based)\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up              - Run all pending migrations\n")
		fmt.Fprintf(os.Stderr, "  down            - Rollback last migration\n")
		fmt.Fprintf(os.Stderr, "  down-to         - Rollback to specific version (requires -target)\n")
		fmt.Fprintf(os.Stderr, "  redo            - Redo last migration (down then up)\n")
		fmt.Fprintf(os.Stderr, "  reset           - Reset all migrations\n")
		fmt.Fprintf(os.Stderr, "  status          - Show migration status\n")
		fmt.Fprintf(os.Stderr, "  version         - Show current migration version\n")
		fmt.Fprintf(os.Stderr, "  create          - Create new migration (requires -name)\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -cmd=up\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=down-to -target=1\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=create -name=add_users_table\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -cmd=create -name=complex_migration -type=go\n", os.Args[0])
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Build Commit: %s\n", BuildCommit)
		fmt.Printf("Build Time: %s\n", BuildTime)
		fmt.Printf("Go Version: %s\n", GoVersion)
		return
	}

	if *command == "" {
		flag.Usage()
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
		dbURL = buildDatabaseURL(cfg)
	}

	if *verbose {
		log.Printf("Connecting to database...")
	}

	// Open database connection
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Set up Goose
	goose.SetBaseFS(nil) // Use filesystem
	goose.SetDialect("postgres")

	if *verbose {
		goose.SetVerbose(true)
	}

	migrationsDir := "migrations"

	// Execute command
	switch *command {
	case "up":
		if err := goose.Up(db, migrationsDir); err != nil {
			log.Fatalf("Failed to run migrations up: %v", err)
		}
		fmt.Println("✓ Migrations completed successfully")

	case "down":
		if err := goose.Down(db, migrationsDir); err != nil {
			log.Fatalf("Failed to run migration down: %v", err)
		}
		fmt.Println("✓ Migration rolled back successfully")

	case "down-to":
		if *target == "" {
			log.Fatal("Target version is required for down-to command")
		}
		version, err := parseVersion(*target)
		if err != nil {
			log.Fatalf("Invalid target version: %v", err)
		}
		if err := goose.DownTo(db, migrationsDir, version); err != nil {
			log.Fatalf("Failed to migrate down to version %d: %v", version, err)
		}
		fmt.Printf("✓ Migrated down to version %d\n", version)

	case "redo":
		if err := goose.Redo(db, migrationsDir); err != nil {
			log.Fatalf("Failed to redo migration: %v", err)
		}
		fmt.Println("✓ Migration redone successfully")

	case "reset":
		if err := goose.Reset(db, migrationsDir); err != nil {
			log.Fatalf("Failed to reset migrations: %v", err)
		}
		fmt.Println("✓ All migrations reset")

	case "status":
		if err := goose.Status(db, migrationsDir); err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}

	case "version":
		version, err := goose.GetDBVersion(db)
		if err != nil {
			log.Fatalf("Failed to get database version: %v", err)
		}
		fmt.Printf("Current database version: %d\n", version)

	case "create":
		if *name == "" {
			log.Fatal("Migration name is required for create command")
		}
		if err := goose.Create(db, migrationsDir, *name, *migType); err != nil {
			log.Fatalf("Failed to create migration: %v", err)
		}
		fmt.Printf("✓ Created %s migration: %s\n", *migType, *name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		flag.Usage()
		os.Exit(1)
	}
}

// buildDatabaseURL constructs the database URL from configuration
func buildDatabaseURL(cfg *config.Config) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)
}

// parseVersion parses a version string and converts it to int64
func parseVersion(versionStr string) (int64, error) {
	if versionStr == "" {
		return 0, fmt.Errorf("version cannot be empty")
	}

	version := int64(0)
	if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
		return 0, fmt.Errorf("invalid version format: %s", versionStr)
	}

	return version, nil
}
