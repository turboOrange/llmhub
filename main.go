package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/joho/godotenv"
    "go.uber.org/zap"
)

type Provider interface {
    Name() string
    Enabled() bool
    Query(ctx context.Context, prompt string, extra map[string]string) (string, error)
}

type OpenAIProvider struct {
    enabled bool
    apiKey  string
}

func (o *OpenAIProvider) Name() string  { return "openai" }
func (o *OpenAIProvider) Enabled() bool { return o.enabled }
func (o *OpenAIProvider) Query(ctx context.Context, prompt string, extra map[string]string) (string, error) {
    // TODO: Integrate with OpenAI API using o.apiKey
    return "OpenAI answer to: " + prompt, nil
}

type Config struct {
    EnabledProviders map[string]bool
    Debug            bool
}

func loadConfig(path string) (*Config, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    dec := json.NewDecoder(f)
    var cfg Config
    if err := dec.Decode(&cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}

func loadAPIKeys(providerNames []string) map[string]string {
    apiKeys := make(map[string]string)
    for _, name := range providerNames {
        envKey := fmt.Sprintf("%s_API_KEY", strings.ToUpper(name))
        apiKey := os.Getenv(envKey)
        apiKeys[name] = apiKey
    }
    return apiKeys
}

func setupLogger(debug bool) (*zap.Logger, error) {
    loggerCfg := zap.NewProductionConfig()
    if debug {
        loggerCfg.Level.SetLevel(zap.DebugLevel)
    }
    return loggerCfg.Build()
}

func getEnabledProviders(cfg *Config, apiKeys map[string]string) []Provider {
    allProviders := []Provider{
        &OpenAIProvider{enabled: cfg.EnabledProviders["openai"], apiKey: apiKeys["openai"]},
        // Add more providers here
    }
    enabled := []Provider{}
    for _, p := range allProviders {
        if p.Enabled() {
            enabled = append(enabled, p)
        }
    }
    return enabled
}

func queryProviders(ctx context.Context, providers []Provider, prompt string, logger *zap.Logger) (map[string]string, map[string]error) {
    var wg sync.WaitGroup
    mu := sync.Mutex{}
    results := make(map[string]string)
    errs := make(map[string]error)

    for _, p := range providers {
        wg.Add(1)
        go func(prov Provider) {
            defer wg.Done()
            answer, err := prov.Query(ctx, prompt, nil)
            mu.Lock()
            defer mu.Unlock()
            if err != nil {
                logger.Error("Provider failed", zap.String("provider", prov.Name()), zap.Error(err))
                errs[prov.Name()] = err
            } else {
                logger.Info("Provider answered", zap.String("provider", prov.Name()))
                results[prov.Name()] = answer
            }
        }(p)
    }
    wg.Wait()
    return results, errs
}

func findSummarizerProvider(providers []Provider, name string) Provider {
    for _, p := range providers {
        if p.Name() == name && p.Enabled() {
            return p
        }
    }
    return nil
}

func summarizeAnswers(ctx context.Context, provider Provider, answers map[string]string, logger *zap.Logger) (string, error) {
    prompt := "Summarize and provide a verdict for these answers:\n"
    for name, answer := range answers {
        prompt += fmt.Sprintf("[%s]: %s\n", name, answer)
    }
    logger.Info("Passing answers to summarizer", zap.String("provider", provider.Name()))
    return provider.Query(ctx, prompt, nil)
}

func main() {
    prompt := flag.String("prompt", "", "Prompt to send to LLMs")
    summarizer := flag.String("summarizer", "openai", "LLM to use for summarizing")
    configPath := flag.String("config", "config.json", "Path to config file")
    envPath := flag.String("env", ".env", "Path to .env file for API keys")
    debug := flag.Bool("debug", false, "Enable debug logging")
    flag.Parse()

    logger, err := setupLogger(*debug)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Logger setup failed: %v\n", err)
        os.Exit(1)
    }
    defer logger.Sync()

    if *prompt == "" {
        logger.Fatal("Prompt is required")
        os.Exit(1)
    }

    cfg, err := loadConfig(*configPath)
    if err != nil {
        logger.Fatal("Failed to load config", zap.Error(err))
        os.Exit(1)
    }
    cfg.Debug = *debug

    // Load env file
    if err := godotenv.Load(*envPath); err != nil {
        logger.Fatal("Failed to load .env file", zap.Error(err))
        os.Exit(1)
    }

    providerNames := make([]string, 0, len(cfg.EnabledProviders))
    for name := range cfg.EnabledProviders {
        providerNames = append(providerNames, name)
    }
    apiKeys := loadAPIKeys(providerNames)

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    enabledProviders := getEnabledProviders(cfg, apiKeys)
    if len(enabledProviders) == 0 {
        logger.Fatal("No enabled LLM providers")
        os.Exit(1)
    }

    logger.Info("Querying providers", zap.Int("count", len(enabledProviders)))
    results, errs := queryProviders(ctx, enabledProviders, *prompt, logger)
    if len(results) == 0 {
        logger.Fatal("No providers returned an answer")
        os.Exit(1)
    }

    summarizerProvider := findSummarizerProvider(enabledProviders, *summarizer)
    if summarizerProvider == nil {
        logger.Fatal("Summarizer provider not found or not enabled", zap.String("summarizer", *summarizer))
        os.Exit(1)
    }

    summary, err := summarizeAnswers(ctx, summarizerProvider, results, logger)
    if err != nil {
        logger.Fatal("Summarizer failed", zap.Error(err))
        os.Exit(1)
    }

    fmt.Println("----- Final Verdict -----")
    fmt.Println(summary)
}