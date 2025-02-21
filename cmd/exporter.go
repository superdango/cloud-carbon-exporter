package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"strings"

	cloudcarbonexporter "github.com/superdango/cloud-carbon-exporter"

	"github.com/superdango/cloud-carbon-exporter/internal/aws"
	"github.com/superdango/cloud-carbon-exporter/internal/demo"
	"github.com/superdango/cloud-carbon-exporter/internal/gcp"
	"github.com/superdango/cloud-carbon-exporter/model"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

func main() {
	ctx := context.Background()

	cloudProvider := ""
	projectID := ""
	listen := ""
	demoEnabled := ""
	logLevel := ""
	logFormat := ""

	flag.StringVar(&cloudProvider, "cloud.provider", "", "cloud provider type (gcp, aws, azure)")
	flag.StringVar(&projectID, "gcp.projectid", "", "gcp project to export data from")
	flag.StringVar(&listen, "listen", "0.0.0.0:2922", "addr to listen to")
	flag.StringVar(&demoEnabled, "demo.enabled", "false", "return fictive demo data")
	flag.StringVar(&logLevel, "log.level", "info", "log severity (debug, info, warn, error)")
	flag.StringVar(&logFormat, "log.format", "text", "log format (text, json)")

	flag.Parse()

	initLogging(logLevel, logFormat)

	params := map[string]string{
		"projectID":    projectID,
		"demo_enabled": demoEnabled,
	}

	collector := cloudcarbonexporter.NewCollector(InitCollectorOptions(ctx, cloudProvider, params)...)

	defer collector.Close()

	mux := http.NewServeMux()
	mux.Handle("/metrics", cloudcarbonexporter.NewOpenMetricsHandler(collector))

	slog.Info("starting cloud carbon exporter", "listen", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
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

func InitCollectorOptions(ctx context.Context, cloudProvider string, params map[string]string) []cloudcarbonexporter.CollectorOptions {
	return []cloudcarbonexporter.CollectorOptions{
		func(c *cloudcarbonexporter.Collector) {
			if params["demo_enabled"] == "true" {
				c.SetOpt(cloudcarbonexporter.WithExplorer(demo.NewExplorer()))
				c.SetOpt(cloudcarbonexporter.WithModels(new(model.Demo)))
			}
		},
		func(c *cloudcarbonexporter.Collector) {
			switch cloudProvider {
			case "gcp":
				if params["projectID"] == "" {
					slog.Error("project id is not set")
					flag.PrintDefaults()
					os.Exit(1)
				}
				model := model.NewGoogleCloudPlatform()
				gcpExplorer, err := gcp.NewExplorer(ctx,
					gcp.WithProjectID(params["projectID"]),
					gcp.WithSupportsFunc(model.Supports),
				)
				if err != nil {
					slog.Error("failed to create gcp explorer", "project_id", params["projectID"], "err", err)
					os.Exit(1)
				}
				c.SetOpt(cloudcarbonexporter.WithExplorer(gcpExplorer))
				c.SetOpt(cloudcarbonexporter.WithModels(model))

			case "aws":
				config, err := config.LoadDefaultConfig(ctx)
				if err != nil {
					slog.Error("failed to create aws explorer", "err", err)
					os.Exit(1)
				}

				model := model.NewAmazonWebServices()
				c.SetOpt(cloudcarbonexporter.WithModels(model))
				c.SetOpt(cloudcarbonexporter.WithExplorer(aws.NewExplorer(ctx,
					aws.Config(config),
					aws.SupportsFunc(model.Supports)),
				))

			case "":
				slog.Error("cloud provider is not set")
				flag.PrintDefaults()
				os.Exit(1)

			default:
				slog.Error("cloud provider is not supported yet", "cloud.provider", cloudProvider)
				os.Exit(1)
			}
		},
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
