# llmhub

A Go CLI tool to query multiple LLMs, summarize with a chosen final LLM, and output their verdict.

## Usage

```sh
go run main.go -prompt "Your question" -summarizer "openai"
```

## Configuration

- **config.json**: Enable/disable LLMs.
- **.env**: Store API keys (`*_API_KEY`).

## Run with GitHub Action

Manual workflow dispatch (`workflow_dispatch`) lets you specify prompt and summarizer.

## Dependencies

- Go 1.21+
- [godotenv](https://github.com/joho/godotenv)
- [zap](https://github.com/uber-go/zap)

Install dependencies:
```sh
go mod tidy
```