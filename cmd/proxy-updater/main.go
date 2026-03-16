package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"proxy-pulse/internal/proxy"
)

func main() {
	cfg := proxy.LoadConfigFromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("proxy-updater failed: %v", err)
	}
}

func run(ctx context.Context, cfg proxy.Config, args []string) error {
	if len(args) == 0 {
		return proxy.Run(ctx, cfg)
	}

	switch args[0] {
	case "discover":
		fs := flag.NewFlagSet("discover", flag.ExitOnError)
		manifestPath := fs.String("manifest", filepath.Join("state", "manifest.json"), "path to write discovery manifest")
		candidatesPath := fs.String("candidates", filepath.Join("state", "candidates.json"), "path to write candidate list")
		_ = fs.Parse(args[1:])

		manifest, candidates, err := proxy.DiscoverCandidates(ctx, cfg)
		if err != nil {
			return err
		}
		if err := proxy.SaveJSON(*manifestPath, manifest); err != nil {
			return err
		}
		return proxy.SaveJSON(*candidatesPath, candidates)
	case "validate-shard":
		fs := flag.NewFlagSet("validate-shard", flag.ExitOnError)
		candidatesPath := fs.String("candidates", filepath.Join("state", "candidates.json"), "path to read candidate list")
		outputPath := fs.String("output", filepath.Join("state", "shard-result.json"), "path to write shard result")
		shardIndex := fs.Int("shard-index", 0, "zero-based shard index")
		shardTotal := fs.Int("shard-total", 1, "total shard count")
		_ = fs.Parse(args[1:])

		candidates, err := proxy.LoadCandidates(*candidatesPath)
		if err != nil {
			return err
		}
		result, err := proxy.ValidateShard(ctx, cfg, candidates, *shardIndex, *shardTotal)
		if err != nil {
			return err
		}
		return proxy.SaveJSON(*outputPath, result)
	case "finalize":
		fs := flag.NewFlagSet("finalize", flag.ExitOnError)
		manifestPath := fs.String("manifest", filepath.Join("state", "manifest.json"), "path to read discovery manifest")
		shardDir := fs.String("shard-dir", filepath.Join("state", "shards"), "directory containing shard results")
		_ = fs.Parse(args[1:])

		manifest, err := proxy.LoadManifest(*manifestPath)
		if err != nil {
			return err
		}
		paths, err := filepath.Glob(filepath.Join(*shardDir, "*.json"))
		if err != nil {
			return err
		}
		results, err := proxy.LoadShardResults(paths)
		if err != nil {
			return err
		}
		return proxy.FinalizeRun(cfg, manifest, results)
	default:
		return proxy.Run(ctx, cfg)
	}
}
