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
	"github.com/superdango/cloud-carbon-exporter/internal/demo"
	"github.com/superdango/cloud-carbon-exporter/internal/gcp"
	"github.com/superdango/cloud-carbon-exporter/internal/scw"
	"github.com/superdango/cloud-carbon-exporter/model/boavizta"
	"github.com/superdango/cloud-carbon-exporter/model/cloudcarbonfootprint"
	modeldemo "github.com/superdango/cloud-carbon-exporter/model/demo"

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
	flagCloudAWSBillingRoleArn := ""
	flagCloudAWSDefaultRegion := ""
	flagListen := ""
	flagDemoEnabled := ""
	flagLogLevel := ""
	flagLogFormat := ""

	flag.StringVar(&flagCloudProvider, "cloud.provider", "", "cloud provider type (gcp, aws, scw)")
	flag.StringVar(&flagCloudGCPProjectID, "cloud.gcp.projectid", "", "gcp project to explore resources from")
	flag.StringVar(&flagCloudAWSRoleArn, "cloud.aws.rolearn", "", "aws role arn to assume")
	flag.StringVar(&flagCloudAWSBillingRoleArn, "cloud.aws.billingrolearn", "", "aws role arn to assume for billing apis")
	flag.StringVar(&flagCloudAWSDefaultRegion, "cloud.aws.defaultregion", "us-east-1", "aws default region")
	flag.StringVar(&flagListen, "listen", "0.0.0.0:2922", "addr to listen to")
	flag.StringVar(&flagDemoEnabled, "demo.enabled", "false", "return fictive demo data")
	flag.StringVar(&flagLogLevel, "log.level", "info", "log severity (debug, info, warn, error)")
	flag.StringVar(&flagLogFormat, "log.format", "text", "log format (text, json)")

	flag.Parse()

	initLogging(flagLogLevel, flagLogFormat)

	collector := cloudcarbonexporter.NewCollector(
		setupCollectorOptions(ctx, map[string]string{
			"cloud.provider":           flagCloudProvider,
			"cloud.gcp.projectid":      flagCloudGCPProjectID,
			"cloud.aws.rolearn":        flagCloudAWSRoleArn,
			"cloud.aws.billingrolearn": flagCloudAWSBillingRoleArn,
			"cloud.aws.defaultregion":  flagCloudAWSDefaultRegion,
			"demo.enabled":             flagDemoEnabled,
		})...)

	defer collector.Close()

	mux := http.NewServeMux()
	mux.Handle("/metrics", cloudcarbonexporter.NewOpenMetricsHandler(collector))

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

func setupCollectorOptions(ctx context.Context, params map[string]string) []cloudcarbonexporter.CollectorOptions {
	return []cloudcarbonexporter.CollectorOptions{
		func(c *cloudcarbonexporter.Collector) {
			if params["demo.enabled"] == "true" {
				c.SetOpt(cloudcarbonexporter.WithExplorer(demo.NewExplorer()))
				c.SetOpt(cloudcarbonexporter.WithModels(new(modeldemo.Demo)))
			}
		},
		func(c *cloudcarbonexporter.Collector) {
			switch params["cloud.provider"] {
			case "gcp":
				if params["cloud.gcp.projectid"] == "" {
					slog.Error("project id is not set")
					flag.PrintDefaults()
					os.Exit(1)
				}
				model := cloudcarbonfootprint.NewGoogleCloudPlatformModel()
				gcpExplorer, err := gcp.NewExplorer(ctx,
					gcp.WithProjectID(params["cloud.gcp.projectid"]),
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
					slog.Error("failed to load aws config", "err", err)
					os.Exit(1)
				}

				awsopts := []aws.ExplorerOption{
					aws.WithAWSConfig(config),
					aws.WithRoleArn(params["cloud.aws.rolearn"]),
					aws.WithBillingRoleArn(params["cloud.aws.billingrolearn"]),
					aws.WithDefaultRegion(params["cloud.aws.defaultregion"]),
				}

				model := cloudcarbonfootprint.NewAWSModel()
				explorer, err := aws.NewExplorer(ctx, awsopts...)
				if err != nil {
					slog.Error("failed to create aws explorer", "err", err)
					os.Exit(1)
				}

				c.SetOpt(cloudcarbonexporter.WithModels(model))
				c.SetOpt(cloudcarbonexporter.WithExplorer(explorer))
			case "scw":
				accessKey := os.Getenv("SCW_ACCESS_KEY")
				secretKey := os.Getenv("SCW_SECRET_KEY")

				client, err := _scw.NewClient(_scw.WithAuth(accessKey, secretKey))
				if err != nil {
					slog.Error("failed to load scaleway client", "err", err)
					os.Exit(1)
				}

				model := boavizta.NewScalewayModel()
				explorer, err := scw.NewExplorer(scw.WithClient(client))
				if err != nil {
					slog.Error("failed to create scaleway explorer", "err", err)
					os.Exit(1)
				}

				c.SetOpt(cloudcarbonexporter.WithModels(model))
				c.SetOpt(cloudcarbonexporter.WithExplorer(explorer))
			case "":
				slog.Error("cloud provider is not set")
				flag.PrintDefaults()
				os.Exit(1)

			default:
				slog.Error("cloud provider is not supported yet", "cloud.provider", params["cloud.provider"])
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
