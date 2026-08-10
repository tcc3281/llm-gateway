package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
	"time"
)

type Config struct {
	ServerURL string
	AdminKey  string
}

func loadCLIConfig() Config {
	url := os.Getenv("GATEWAY_URL")
	if url == "" {
		url = "http://localhost:8081"
	}
	key := os.Getenv("ADMIN_API_KEY")
	if key == "" {
		key = "sk-admin-secret-key-change-me"
	}
	return Config{ServerURL: url, AdminKey: key}
}


func main() {
	if len(os.Args) < 2 {
		printGlobalHelp()
		os.Exit(1)
	}

	cfg := loadCLIConfig()
	subcommand := os.Args[1]

	switch subcommand {
	case "health":
		handleHealth(cfg)
	case "user":
		handleUserCommands(cfg, os.Args[2:])
	case "key":
		handleKeyCommands(cfg, os.Args[2:])
	case "model":
		handleModelCommands(cfg, os.Args[2:])
	case "provider":
		handleProviderCommands(cfg, os.Args[2:])
	case "stats":
		handleStatsCommands(cfg, os.Args[2:])
	case "version":
		fmt.Println("LLM Gateway CLI v1.0.0")
	case "help", "-h", "--help":
		printGlobalHelp()
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", subcommand)
		printGlobalHelp()
		os.Exit(1)
	}
}

func printGlobalHelp() {
	fmt.Println(`LLM Gateway Command Line Interface (llmgw)

Usage:
  llmgw [command] [subcommand] [flags]

Available Commands:
  health      Check LLM Gateway server connectivity and status
  user        Manage users (create, list)
  key         Manage API keys (create, list, revoke)
  provider    Manage upstream provider API keys (set, list, delete)
  model       Manage model pricing and routing
  stats       View timeseries usage analytics and telemetry
  version     Show CLI version

Flags:
  -h, --help  Show help for command

Environment Variables:
  GATEWAY_URL    Admin server URL (default: http://localhost:8081)
  ADMIN_API_KEY  Admin secret key for authentication (default: sk-admin-secret-key-change-me)

Examples:
  llmgw health
  llmgw user create --email dev@example.com --name "Developer 1"
  llmgw user list
  llmgw key create --user-id <uuid> --name "Dev Key" --budget 50.0 --rpm 120
  llmgw key list
  llmgw key revoke --id <key-uuid>
  llmgw provider set --provider openai --api-key sk-proj-...
  llmgw provider list
  llmgw model list
  llmgw stats --interval hour`)
}

// ----------------------------------------------------
// Health Command
// ----------------------------------------------------
func handleHealth(cfg Config) {
	resp, err := sendRequest(cfg, "GET", "/health", nil)
	if err != nil {
		fmt.Printf("❌ Failed connecting to Gateway at %s: %v\n", cfg.ServerURL, err)
		os.Exit(1)
	}
	fmt.Printf("✅ Gateway is healthy! Server: %s\nResponse: %s\n", cfg.ServerURL, string(resp))
}

// ----------------------------------------------------
// User Commands
// ----------------------------------------------------
func handleUserCommands(cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: llmgw user [create|list]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		res, err := sendRequest(cfg, "GET", "/admin/users", nil)
		if err != nil {
			fmt.Printf("❌ Failed listing users: %v\n", err)
			os.Exit(1)
		}

		var users []map[string]interface{}
		if err := json.Unmarshal(res, &users); err != nil {
			fmt.Println(string(res))
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tCREATED AT")
		fmt.Fprintln(w, "--\t-----\t----\t----\t----------")
		for _, u := range users {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", u["id"], u["email"], u["name"], u["role"], u["created_at"])
		}
		w.Flush()

	case "create":
		fs := flag.NewFlagSet("user create", flag.ExitOnError)
		email := fs.String("email", "", "User email address (required)")
		name := fs.String("name", "", "User full name (required)")
		role := fs.String("role", "user", "User role (admin or user)")
		fs.Parse(args[1:])

		if *email == "" || *name == "" {
			fmt.Println("Error: --email and --name flags are required.")
			fs.Usage()
			os.Exit(1)
		}

		payload := map[string]string{
			"email": *email,
			"name":  *name,
			"role":  *role,
		}

		res, err := sendRequest(cfg, "POST", "/admin/users", payload)
		if err != nil {
			fmt.Printf("❌ Failed to create user: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ User Created Successfully:")
		fmt.Println(string(res))

	default:
		fmt.Printf("Unknown user command '%s'\n", args[0])
	}
}

// ----------------------------------------------------
// Key Commands
// ----------------------------------------------------
func handleKeyCommands(cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: llmgw key [create|list|revoke]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		res, err := sendRequest(cfg, "GET", "/admin/keys", nil)
		if err != nil {
			fmt.Printf("❌ Failed listing API keys: %v\n", err)
			os.Exit(1)
		}

		var keys []map[string]interface{}
		if err := json.Unmarshal(res, &keys); err != nil {
			fmt.Println(string(res))
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tUSER ID\tNAME\tPREFIX\tBUDGET (USD)\tRPM\tACTIVE")
		fmt.Fprintln(w, "--\t-------\t----\t------\t------------\t---\t------")
		for _, k := range keys {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\t$%v\t%v\t%v\n",
				k["id"], k["user_id"], k["name"], k["key_prefix"], k["budget_limit"], k["rate_limit_rpm"], k["is_active"])
		}
		w.Flush()

	case "revoke":
		fs := flag.NewFlagSet("key revoke", flag.ExitOnError)
		id := fs.String("id", "", "API key UUID to revoke (required)")
		fs.Parse(args[1:])

		if *id == "" {
			fmt.Println("Error: --id flag is required.")
			os.Exit(1)
		}

		res, err := sendRequest(cfg, "DELETE", fmt.Sprintf("/admin/keys?id=%s", *id), nil)
		if err != nil {
			fmt.Printf("❌ Failed revoking key: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(res))

	case "create":
		fs := flag.NewFlagSet("key create", flag.ExitOnError)
		userID := fs.String("user-id", "", "User UUID (required)")
		name := fs.String("name", "", "Key name/description (required)")
		budget := fs.Float64("budget", 0.0, "Budget limit in USD (0 = unlimited)")
		rpm := fs.Int("rpm", 60, "Rate limit in requests per minute")
		fs.Parse(args[1:])

		if *userID == "" || *name == "" {
			fmt.Println("Error: --user-id and --name flags are required.")
			fs.Usage()
			os.Exit(1)
		}

		payload := map[string]interface{}{
			"user_id":        *userID,
			"name":           *name,
			"budget_limit":   *budget,
			"rate_limit_rpm": *rpm,
		}

		res, err := sendRequest(cfg, "POST", "/admin/keys", payload)
		if err != nil {
			fmt.Printf("❌ Failed to create key: %v\n", err)
			os.Exit(1)
		}

		var result map[string]interface{}
		json.Unmarshal(res, &result)

		fmt.Println("🔑 API Key Created Successfully!")
		fmt.Println("--------------------------------------------------")
		fmt.Printf("Secret Key (SAVE THIS NOW!): %v\n", result["api_key"])
		fmt.Println("--------------------------------------------------")

	default:
		fmt.Printf("Unknown key command '%s'\n", args[0])
	}
}

// ----------------------------------------------------
// Model Commands
// ----------------------------------------------------
func handleModelCommands(cfg Config, args []string) {
	if len(args) == 0 {
		handleModelList(cfg)
		return
	}

	switch args[0] {
	case "list":
		handleModelList(cfg)
	case "add":
		fs := flag.NewFlagSet("model add", flag.ExitOnError)
		provider := fs.String("provider", "", "Provider (openai, anthropic, gemini) (required)")
		name := fs.String("name", "", "Model name (e.g. gpt-4o, claude-3-5-sonnet-20240620) (required)")
		promptPrice := fs.Float64("prompt-price", 0.0, "Price per 1M prompt tokens in USD")
		completionPrice := fs.Float64("completion-price", 0.0, "Price per 1M completion tokens in USD")
		fs.Parse(args[1:])

		if *provider == "" || *name == "" {
			fmt.Println("Error: --provider and --name flags are required.")
			fs.Usage()
			os.Exit(1)
		}

		payload := map[string]interface{}{
			"provider":                *provider,
			"model_name":              *name,
			"prompt_price_per_1m":     *promptPrice,
			"completion_price_per_1m": *completionPrice,
		}

		res, err := sendRequest(cfg, "POST", "/admin/models", payload)
		if err != nil {
			fmt.Printf("❌ Failed adding model: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Model pricing saved:")
		fmt.Println(string(res))

	case "delete":
		fs := flag.NewFlagSet("model delete", flag.ExitOnError)
		id := fs.String("id", "", "Model UUID to delete (required)")
		fs.Parse(args[1:])

		if *id == "" {
			fmt.Println("Error: --id flag is required.")
			os.Exit(1)
		}

		res, err := sendRequest(cfg, "DELETE", fmt.Sprintf("/admin/models?id=%s", *id), nil)
		if err != nil {
			fmt.Printf("❌ Failed deleting model: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(res))

	default:
		fmt.Printf("Unknown model command '%s'\n", args[0])
	}
}

func handleModelList(cfg Config) {
	res, err := sendRequest(cfg, "GET", "/admin/models", nil)
	if err != nil {
		fmt.Printf("❌ Failed listing models: %v\n", err)
		os.Exit(1)
	}

	var models []map[string]interface{}
	if err := json.Unmarshal(res, &models); err != nil {
		fmt.Println(string(res))
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tPROVIDER\tMODEL NAME\tPROMPT / 1M\tCOMPLETION / 1M")
	fmt.Fprintln(w, "--\t--------\t----------\t-----------\t---------------")
	for _, m := range models {
		fmt.Fprintf(w, "%v\t%v\t%v\t$%v\t$%v\n",
			m["id"], m["provider"], m["model_name"], m["prompt_price_per_1m"], m["completion_price_per_1m"])
	}
	w.Flush()
}

// ----------------------------------------------------
// Provider Key Commands
// ----------------------------------------------------
func handleProviderCommands(cfg Config, args []string) {
	if len(args) == 0 {
		handleProviderList(cfg)
		return
	}

	switch args[0] {
	case "list":
		handleProviderList(cfg)
	case "set":
		fs := flag.NewFlagSet("provider set", flag.ExitOnError)
		providerName := fs.String("provider", "", "Provider (openai, anthropic, gemini) (required)")
		apiKey := fs.String("api-key", "", "Upstream API key (required)")
		fs.Parse(args[1:])

		if *providerName == "" || *apiKey == "" {
			fmt.Println("Error: --provider and --api-key flags are required.")
			os.Exit(1)
		}

		payload := map[string]string{
			"provider": *providerName,
			"api_key":  *apiKey,
		}
		res, err := sendRequest(cfg, "POST", "/admin/providers", payload)
		if err != nil {
			fmt.Printf("❌ Failed saving provider key: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Provider API key saved:")
		fmt.Println(string(res))

	case "delete":
		fs := flag.NewFlagSet("provider delete", flag.ExitOnError)
		providerName := fs.String("provider", "", "Provider to delete (required)")
		fs.Parse(args[1:])

		if *providerName == "" {
			fmt.Println("Error: --provider flag is required.")
			os.Exit(1)
		}

		res, err := sendRequest(cfg, "DELETE", fmt.Sprintf("/admin/providers?provider=%s", *providerName), nil)
		if err != nil {
			fmt.Printf("❌ Failed deleting provider key: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(res))

	default:
		fmt.Printf("Unknown provider command '%s'\n", args[0])
	}
}

func handleProviderList(cfg Config) {
	res, err := sendRequest(cfg, "GET", "/admin/providers", nil)
	if err != nil {
		fmt.Printf("❌ Failed listing providers: %v\n", err)
		os.Exit(1)
	}

	var providers []map[string]interface{}
	if err := json.Unmarshal(res, &providers); err != nil {
		fmt.Println(string(res))
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tKEY PREVIEW\tACTIVE\tUPDATED AT")
	fmt.Fprintln(w, "--------\t-----------\t------\t----------")
	for _, p := range providers {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", p["provider"], p["key_preview"], p["is_active"], p["updated_at"])
	}
	w.Flush()
}

// ----------------------------------------------------
// Stats Commands
// ----------------------------------------------------
func handleStatsCommands(cfg Config, args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	interval := fs.String("interval", "hour", "Time interval (hour or day)")
	userID := fs.String("user-id", "", "Filter stats by user UUID")
	fs.Parse(args)

	path := fmt.Sprintf("/admin/analytics/stats?interval=%s", *interval)
	if *userID != "" {
		path += "&user_id=" + *userID
	}

	res, err := sendRequest(cfg, "GET", path, nil)
	if err != nil {
		fmt.Printf("❌ Failed fetching stats: %v\n", err)
		os.Exit(1)
	}

	var statsResp map[string]interface{}
	json.Unmarshal(res, &statsResp)

	fmt.Println("📊 LLM Gateway Usage & Analytics Report")
	fmt.Printf("Interval: %v | From: %v | To: %v\n\n", statsResp["interval"], statsResp["from"], statsResp["to"])

	data, ok := statsResp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Println("No telemetry records found for this period.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME BUCKET\tPROVIDER\tMODEL\tREQUESTS\tPROMPT TOKENS\tCOMPLETION TOKENS\tTOTAL COST\tAVG TTFT\tAVG LATENCY")
	fmt.Fprintln(w, "-----------\t--------\t-----\t--------\t-------------\t-----------------\t----------\t--------\t-----------")
	for _, item := range data {
		m := item.(map[string]interface{})
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t$%v\t%v ms\t%v ms\n",
			m["time_bucket"], m["provider"], m["model"], m["total_requests"],
			m["sum_prompt_tokens"], m["sum_completion_tokens"], m["total_cost"],
			m["avg_ttft_ms"], m["avg_latency_ms"])
	}
	w.Flush()
}

// ----------------------------------------------------
// Helper HTTP Sender
// ----------------------------------------------------
func sendRequest(cfg Config, method, path string, bodyPayload interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if bodyPayload != nil {
		jsonBytes, err := json.Marshal(bodyPayload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(jsonBytes)
	}

	url := cfg.ServerURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-API-Key", cfg.AdminKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	resBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(resBytes))
	}

	return resBytes, nil
}
