package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	_scw "github.com/scaleway/scaleway-sdk-go/scw"
	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"

	"github.com/superdango/cloud-carbon-exporter/internal/aws"
	"github.com/superdango/cloud-carbon-exporter/internal/gcp"
	"github.com/superdango/cloud-carbon-exporter/internal/scw"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

func main() {
	ctx := context.Background()

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

		flag.PrintDefaults()

		fmt.Fprint(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprint(os.Stderr, "  SCW_ACCESS_KEY\n")
		fmt.Fprint(os.Stderr, "        scaleway access key\n")
		fmt.Fprint(os.Stderr, "  SCW_SECRET_KEY\n")
		fmt.Fprint(os.Stderr, "        scaleway secret key\n")
	}

	flagCloudProvider := ""
	flagCloudGCPProjectID := ""
	flagCloudAWSRoleArn := ""
	flagCloudAWSDefaultRegion := ""
	flagListen := ""
	flagLogLevel := ""
	flagLogFormat := ""

	flag.StringVar(&flagCloudProvider, "cloud.provider", "", "cloud provider type (gcp, aws, scw)")
	flag.StringVar(&flagCloudGCPProjectID, "cloud.gcp.projectid", "", "gcp project to explore resources from")
	flag.StringVar(&flagCloudAWSRoleArn, "cloud.aws.rolearn", "", "aws role arn to assume")
	flag.StringVar(&flagCloudAWSDefaultRegion, "cloud.aws.defaultregion", "us-east-1", "aws default region")
	flag.StringVar(&flagListen, "listen", "0.0.0.0:2922", "addr to listen to")
	flag.StringVar(&flagLogLevel, "log.level", "info", "log severity (debug, info, warn, error)")
	flag.StringVar(&flagLogFormat, "log.format", "text", "log format (text, json)")

	flag.Parse()

	initLogging(flagLogLevel, flagLogFormat)

	explorer, err := setupExplorer(ctx, map[string]string{
		"cloud.provider":          flagCloudProvider,
		"cloud.gcp.projectid":     flagCloudGCPProjectID,
		"cloud.aws.rolearn":       flagCloudAWSRoleArn,
		"cloud.aws.defaultregion": flagCloudAWSDefaultRegion,
	})
	if err != nil {
		slog.Error("failed to create explorer", "err", err.Error())
		os.Exit(1)
	}
	defer explorer.Close()

	mux := http.NewServeMux()
	mux.Handle("/metrics", cloudcarbonexporter.NewOpenMetricsHandler(explorer))

	slog.Info("starting cloud carbon exporter", "listen", flagListen)
	if err := http.ListenAndServe(flagListen, mux); err != nil {
		slog.Error("failed to start cloud carbon exporter", "err", err)
		os.Exit(1)
	}
}

func initLogging(logLevel string, logFormat string) {
	switch logFormat {
	case "text":
		slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
			Level:   slogLevel(logLevel),
			NoColor: !isatty.IsTerminal(os.Stdout.Fd()),
		})))
	case "json":
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slogLevel(logLevel),
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				switch a.Key {
				case slog.LevelKey:
					a.Key = "severity"
					return a
				case slog.MessageKey:
					a.Key = "message"
					return a
				default:
					return a
				}
			},
		})))
	}
}

func setupExplorer(ctx context.Context, params map[string]string) (cloudcarbonexporter.Explorer, error) {
	switch params["cloud.provider"] {
	case "gcp":
		if params["cloud.gcp.projectid"] == "" {
			slog.Error("project id is not set")
			flag.PrintDefaults()
			os.Exit(1)
		}
		return gcp.NewExplorer(ctx,
			gcp.WithProjectID(params["cloud.gcp.projectid"]),
		)

	case "aws":
		config, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			slog.Error("failed to load aws config", "err", err)
			os.Exit(1)
		}

		awsopts := []aws.ExplorerOption{
			aws.WithAWSConfig(config),
			aws.WithRoleArn(params["cloud.aws.rolearn"]),
			aws.WithDefaultRegion(params["cloud.aws.defaultregion"]),
		}

		return aws.NewExplorer(ctx, awsopts...)

	case "scw":
		accessKey := os.Getenv("SCW_ACCESS_KEY")
		secretKey := os.Getenv("SCW_SECRET_KEY")

		client, err := _scw.NewClient(_scw.WithAuth(accessKey, secretKey))
		if err != nil {
			return nil, fmt.Errorf("failed to load scaleway client: %w", err)
		}

		return scw.NewExplorer(scw.WithClient(client))

	case "":
		return nil, fmt.Errorf("cloud provider is not set")

	default:
		return nil, fmt.Errorf("cloud provider %s is not supported", params["cloud.provider"])
	}
}

func slogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}

	return slog.LevelInfo
}
