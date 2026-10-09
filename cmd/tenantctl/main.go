package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lib/pq"

	"wobble/internal/auth"
	"wobble/internal/config"
	"wobble/internal/database"
	"wobble/internal/tenant"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "add":
		handleAdd(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("TechNova Tenant Management CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  tenantctl add --key <key_identifier> --app <app_name> --name <tenant_name> [--origin <allowed_origins>]")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  go run ./cmd/tenantctl add --key kemkes_1101015 --app \"KlikMedic SIMRS\" --name \"RSUD Meuraxa\" --origin \"http://localhost:8080,https://simrs.rsudmeuraxa.go.id\"")
}

func handleAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	keyIdentifier := fs.String("key", "", "Unique key identifier for tenant (e.g. kemkes_1101015)")
	appName := fs.String("app", "", "Application name (e.g. KlikMedic SIMRS)")
	tenantName := fs.String("name", "", "Tenant / Hospital name (e.g. RSUD Meuraxa)")
	originsFlag := fs.String("origin", "*", "Allowed CORS/WS origins comma-separated (default: *)")

	_ = fs.Parse(args)

	if strings.TrimSpace(*keyIdentifier) == "" || strings.TrimSpace(*appName) == "" || strings.TrimSpace(*tenantName) == "" {
		fmt.Fprintln(os.Stderr, "Error: --key, --app, and --name are required.")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewPostgresDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	repo := tenant.NewRepository(db)

	rawAPIKey, hash, err := auth.GenerateAPIKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating API key: %v\n", err)
		os.Exit(1)
	}

	var origins []string
	for _, o := range strings.Split(*originsFlag, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	t := &tenant.Tenant{
		KeyIdentifier:  strings.TrimSpace(*keyIdentifier),
		AppName:        strings.TrimSpace(*appName),
		TenantName:     strings.TrimSpace(*tenantName),
		APIKeyHash:     hash,
		AllowedOrigins: pq.StringArray(origins),
		IsActive:       true,
	}

	if err := repo.Create(context.Background(), t); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tenant in database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(" Tenant successfully registered!")
	fmt.Printf("ID             : %d\n", t.ID)
	fmt.Printf("Key Identifier : %s\n", t.KeyIdentifier)
	fmt.Printf("App Name       : %s\n", t.AppName)
	fmt.Printf("Tenant Name    : %s\n", t.TenantName)
	fmt.Printf("Allowed Origins: %v\n", t.AllowedOrigins)
	fmt.Println("-----------------------------------------------------------------")
	fmt.Println(" API Key (SAVE THIS NOW, it is never stored or displayed again!):")
	fmt.Println(rawAPIKey)
}
