package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

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
	flagPrintSupportedServices := ""

	flag.StringVar(&flagCloudProvider, "cloud.provider", "", "cloud provider type (gcp, aws, scw)")
	flag.StringVar(&flagCloudGCPProjectID, "cloud.gcp.projectid", "", "gcp project to explore resources from")
	flag.StringVar(&flagCloudAWSRoleArn, "cloud.aws.rolearn", "", "aws role arn to assume")
	flag.StringVar(&flagCloudAWSDefaultRegion, "cloud.aws.defaultregion", "us-east-1", "aws default region")
	flag.StringVar(&flagListen, "listen", "0.0.0.0:2922", "addr to listen to")
	flag.StringVar(&flagLogLevel, "log.level", "info", "log severity (debug, info, warn, error)")
	flag.StringVar(&flagLogFormat, "log.format", "text", "log format (text, json)")
	flag.StringVar(&flagPrintSupportedServices, "print-supported-services", "", "print on stdout the supported services list (markdown)")

	flag.Parse()

	initLogging(flagLogLevel, flagLogFormat)

	configmap := map[string]string{
		"cloud.provider":          flagCloudProvider,
		"cloud.gcp.projectid":     flagCloudGCPProjectID,
		"cloud.aws.rolearn":       flagCloudAWSRoleArn,
		"cloud.aws.defaultregion": flagCloudAWSDefaultRegion,
	}

	explorers := map[string]cloudcarbonexporter.Explorer{
		"aws": aws.NewExplorer(),
		"gcp": gcp.NewExplorer(),
		"scw": scw.NewExplorer(),
	}

	if flagPrintSupportedServices == "markdown" {
		printMarkdownSupportedServices(explorers)
		os.Exit(0)
	}

	explorer, found := explorers[configmap["cloud.provider"]]
	if !found {
		slog.Error("explorer not supported", "explorer", configmap["cloud.provider"])
		os.Exit(1)
	}

	err := initExplorer(ctx, explorer, configmap)
	if err != nil {
		slog.Error("failed to init explorer", "err", err.Error())
		os.Exit(1)
	}
	defer explorer.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<a href=\"/metrics\">go to /metrics</a>")
	})
	mux.Handle("/metrics", cloudcarbonexporter.NewOpenMetricsHandler(explorer))

	slog.Info("starting cloud carbon exporter", "listen", "http://"+flagListen)
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

func initExplorer(ctx context.Context, explorer cloudcarbonexporter.Explorer, params map[string]string) error {
	switch params["cloud.provider"] {
	case "gcp":
		gcpExplorer := explorer.(*gcp.Explorer)
		gcpExplorer.ProjectID = params["cloud.gcp.projectid"]
		return gcpExplorer.Init(ctx)

	case "aws":
		awsExplorer := explorer.(*aws.Explorer)

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

		return awsExplorer.Configure(awsopts...).Init(ctx)

	case "scw":
		scwExplorer := explorer.(*scw.Explorer)

		accessKey := os.Getenv("SCW_ACCESS_KEY")
		secretKey := os.Getenv("SCW_SECRET_KEY")

		client, err := _scw.NewClient(_scw.WithAuth(accessKey, secretKey))
		if err != nil {
			return fmt.Errorf("failed to load scaleway client: %w", err)
		}

		return scwExplorer.Configure(scw.WithClient(client)).Init(ctx)

	case "":
		return fmt.Errorf("cloud provider is not set")

	default:
		return fmt.Errorf("cloud provider %s is not supported", params["cloud.provider"])
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

func printMarkdownSupportedServices(explorers map[string]cloudcarbonexporter.Explorer) {
	str := ""
	for explorerName, explorer := range explorers {
		str += "## " + strings.ToUpper(explorerName) + "\n\n"
		for _, service := range explorer.SupportedServices() {
			str += "* `" + service + "`\n"
		}
		if len(explorer.SupportedServices()) == 0 {
			str += "* `none` \n"
		}
		str += "\n"
	}
	str += fmt.Sprintf("_automatically generated on %s_", time.Now().UTC().Format(time.RFC850))

	fmt.Println(str)
}
