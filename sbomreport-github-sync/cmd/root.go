package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	githubsync "github.com/ystkfujii/playground_supply-chain-guard/sbomreport-github-sync/internal/github"
	"github.com/ystkfujii/playground_supply-chain-guard/sbomreport-github-sync/internal/kube"
	"github.com/ystkfujii/playground_supply-chain-guard/sbomreport-github-sync/internal/render"
)

type syncOptions struct {
	Kubeconfig string
	Namespace  string
	Selector   string

	GitHubToken      string
	GitHubOwner      string
	GitHubRepo       string
	GitHubBranch     string
	GitHubAPIURL     string
	GitHubAPIVersion string

	ClusterName   string
	PathPrefix    string
	CommitMessage string
	Content       string

	IncludeIndex  bool
	DeleteMissing bool
	FailIfEmpty   bool
	DryRun        bool
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "sbomreport-github-sync",
		Short:         "Sync Trivy Operator SbomReport resources to a GitHub repository",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	syncOpts := &syncOptions{}
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "List SbomReports from Kubernetes and commit them to GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd.Context(), syncOpts)
		},
	}

	flags := syncCmd.Flags()
	flags.StringVar(&syncOpts.Kubeconfig, "kubeconfig", envOrDefault("KUBECONFIG", ""), "Path to kubeconfig. Empty means in-cluster config first, then ~/.kube/config")
	flags.StringVar(&syncOpts.Namespace, "namespace", envOrDefault("NAMESPACE", ""), "Namespace to list SbomReports from. Empty means all namespaces")
	flags.StringVar(&syncOpts.Selector, "selector", envOrDefault("LABEL_SELECTOR", ""), "Kubernetes label selector for SbomReports")

	flags.StringVar(&syncOpts.GitHubToken, "github-token", envOrDefault("GITHUB_TOKEN", ""), "GitHub token. Prefer GITHUB_TOKEN env")
	flags.StringVar(&syncOpts.GitHubOwner, "github-owner", envOrDefault("GITHUB_OWNER", ""), "GitHub repository owner")
	flags.StringVar(&syncOpts.GitHubRepo, "github-repo", envOrDefault("GITHUB_REPO", ""), "GitHub repository name")
	flags.StringVar(&syncOpts.GitHubBranch, "github-branch", envOrDefault("GITHUB_BRANCH", "main"), "GitHub branch to update")
	flags.StringVar(&syncOpts.GitHubAPIURL, "github-api-url", envOrDefault("GITHUB_API_URL", "https://api.github.com"), "GitHub API URL. For GitHub Enterprise, set the API base URL")
	flags.StringVar(&syncOpts.GitHubAPIVersion, "github-api-version", envOrDefault("GITHUB_API_VERSION", "2026-03-10"), "GitHub REST API version header")

	flags.StringVar(&syncOpts.ClusterName, "cluster-name", envOrDefault("CLUSTER_NAME", ""), "Cluster name included in index metadata")
	flags.StringVar(&syncOpts.PathPrefix, "path-prefix", envOrDefault("GITHUB_PATH_PREFIX", "sbomreports"), "Repository directory prefix for generated files")
	flags.StringVar(&syncOpts.CommitMessage, "commit-message", envOrDefault("COMMIT_MESSAGE", ""), "Commit message. Empty means generated message")
	flags.StringVar(&syncOpts.Content, "content", envOrDefault("SBOMREPORT_CONTENT", "cyclonedx"), "Content to write: cyclonedx, report, or resource")

	flags.BoolVar(&syncOpts.IncludeIndex, "include-index", envBoolOrDefault("INCLUDE_INDEX", true), "Write an index.json file under path-prefix")
	flags.BoolVar(&syncOpts.DeleteMissing, "delete-missing", envBoolOrDefault("DELETE_MISSING", true), "Delete stale .json files under path-prefix that are not present in the current Kubernetes result")
	flags.BoolVar(&syncOpts.FailIfEmpty, "fail-if-empty", envBoolOrDefault("FAIL_IF_EMPTY", false), "Return an error if no SbomReports are found")
	flags.BoolVar(&syncOpts.DryRun, "dry-run", false, "List and render reports but do not call GitHub")

	root.AddCommand(syncCmd)
	return root
}

func runSync(ctx context.Context, opts *syncOptions) error {
	opts.Content = strings.ToLower(strings.TrimSpace(opts.Content))
	if opts.Content != "cyclonedx" && opts.Content != "report" && opts.Content != "resource" {
		return fmt.Errorf("--content must be one of: cyclonedx, report, resource")
	}
	if opts.DeleteMissing && strings.Trim(opts.PathPrefix, "/") == "" {
		return fmt.Errorf("--delete-missing requires a non-empty --path-prefix to avoid deleting unrelated repository files")
	}
	if !opts.DryRun {
		if opts.GitHubToken == "" {
			return fmt.Errorf("GITHUB_TOKEN or --github-token is required")
		}
		if opts.GitHubOwner == "" || opts.GitHubRepo == "" {
			return fmt.Errorf("GITHUB_OWNER/GITHUB_REPO or --github-owner/--github-repo are required")
		}
	}

	kubeClient, err := kube.NewDynamicClient(opts.Kubeconfig)
	if err != nil {
		return err
	}

	reports, err := kube.ListSbomReports(ctx, kubeClient, opts.Namespace, opts.Selector)
	if err != nil {
		return err
	}
	if len(reports) == 0 && opts.FailIfEmpty {
		return fmt.Errorf("no SbomReports found")
	}

	files, err := render.RenderSbomReports(reports, render.Options{
		PathPrefix:   opts.PathPrefix,
		Content:      opts.Content,
		ClusterName:  opts.ClusterName,
		IncludeIndex: opts.IncludeIndex,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "found %d SbomReports, rendered %d files\n", len(reports), len(files))
	for _, file := range files {
		fmt.Fprintf(os.Stderr, "  %s (%d bytes)\n", file.Path, len(file.Content))
	}

	if opts.DryRun {
		return nil
	}

	message := opts.CommitMessage
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("Sync Trivy SbomReports (%s)", time.Now().UTC().Format(time.RFC3339))
	}

	gh := githubsync.NewClient(opts.GitHubAPIURL, opts.GitHubToken, opts.GitHubAPIVersion)
	result, err := gh.CommitFiles(ctx, githubsync.CommitRequest{
		Owner:         opts.GitHubOwner,
		Repo:          opts.GitHubRepo,
		Branch:        opts.GitHubBranch,
		Message:       message,
		Files:         files,
		DeleteMissing: opts.DeleteMissing,
		PathPrefix:    opts.PathPrefix,
	})
	if err != nil {
		return err
	}
	if result.Skipped {
		fmt.Fprintln(os.Stderr, "no changes; skipped commit")
		return nil
	}
	fmt.Fprintf(os.Stderr, "created commit %s\n", result.CommitSHA)
	if result.CommitURL != "" {
		fmt.Fprintf(os.Stderr, "%s\n", result.CommitURL)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
