package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/kubescape/synchronizer/adapters"
	"github.com/kubescape/synchronizer/core"
	"github.com/kubescape/synchronizer/domain"
)

const (
	defaultPort               = "8080"
	gitHubAPIVersion          = "2022-11-28"
	defaultPathPrefix         = "kubescape-sbom"
	defaultBatchMaxItems      = 20
	defaultBatchQueueSize     = 1000
	defaultBatchFlushInterval = 10 * time.Second
	defaultGitHubAPITimeout   = 120 * time.Second
)

type Config struct {
	Port                     string
	ProviderAccessKey        string
	GitHubToken              string
	GitHubOwner              string
	GitHubRepo               string
	GitHubBranch             string
	PathPrefix               string
	SynchronizerURL          string
	TLSCertFile              string
	TLSKeyFile               string
	GitHubBatchMaxItems      int
	GitHubBatchQueueSize     int
	GitHubBatchFlushInterval time.Duration
	GitHubAPITimeout         time.Duration
}

type Server struct {
	cfg    Config
	github *GitHubClient
}

type GitHubAdapter struct {
	cfg       Config
	github    *GitHubClient
	callbacks domain.Callbacks
}

type SBOMMeta struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Metadata   struct {
		Name        string            `json:"name,omitempty"`
		Namespace   string            `json:"namespace,omitempty"`
		Annotations map[string]string `json:"annotations,omitempty"`
		Labels      map[string]string `json:"labels,omitempty"`
	} `json:"metadata,omitempty"`
}

type GitHubClient struct {
	baseURL    string
	token      string
	owner      string
	repo       string
	branch     string
	http       *http.Client
	apiTimeout time.Duration
	mu         sync.Mutex
	batcher    *GitHubBatcher
	startOnce  sync.Once
	startErr   error
}

type GitHubFileUpdate struct {
	Path    string
	Content []byte
	Source  string
}

type GitHubBatcher struct {
	github        *GitHubClient
	maxItems      int
	flushInterval time.Duration
	queue         chan GitHubFileUpdate
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
}

type serviceDiscoveryResponse struct {
	Version  string            `json:"version"`
	Response map[string]string `json:"response"`
}

type gitRefResponse struct {
	Ref    string `json:"ref,omitempty"`
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type,omitempty"`
	} `json:"object"`
}

type gitCommitResponse struct {
	SHA  string `json:"sha"`
	Tree struct {
		SHA string `json:"sha"`
	} `json:"tree"`
}

type createBlobRequest struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type createBlobResponse struct {
	SHA string `json:"sha"`
}

type createTreeRequest struct {
	BaseTree string         `json:"base_tree,omitempty"`
	Tree     []gitTreeEntry `json:"tree"`
}

type gitTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type createTreeResponse struct {
	SHA string `json:"sha"`
}

type createCommitRequest struct {
	Message string   `json:"message"`
	Tree    string   `json:"tree"`
	Parents []string `json:"parents"`
}

type createCommitResponse struct {
	SHA string `json:"sha"`
}

type updateRefRequest struct {
	SHA   string `json:"sha"`
	Force bool   `json:"force"`
}

type GitHubAPIError struct {
	Operation  string
	StatusCode int
	Body       string
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub %s failed status=%d body=%s", e.Operation, e.StatusCode, e.Body)
}

var _ adapters.Adapter = (*GitHubAdapter)(nil)

func main() {
	cfg := loadConfig()
	github := NewGitHubClient(cfg)

	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := github.Start(runCtx); err != nil {
		log.Fatal(err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), cfg.GitHubAPITimeout)
		defer cancel()
		if err := github.Stop(stopCtx); err != nil {
			log.Printf("stop GitHub batcher: %v", err)
		}
	}()

	s := &Server{cfg: cfg, github: github}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/api/v3/servicediscovery", s.handleServiceDiscovery)
	mux.HandleFunc("/", s.handleSynchronizer)

	addr := ":" + cfg.Port
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf(
			"kubescape-github-provider listening on %s, repo=%s/%s, branch=%s, prefix=%s, batchMaxItems=%d, batchFlushInterval=%s",
			addr,
			cfg.GitHubOwner,
			cfg.GitHubRepo,
			cfg.GitHubBranch,
			cfg.PathPrefix,
			cfg.GitHubBatchMaxItems,
			cfg.GitHubBatchFlushInterval,
		)
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			errCh <- server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-runCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown HTTP server: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}

func NewGitHubClient(cfg Config) *GitHubClient {
	client := &GitHubClient{
		baseURL:    "https://api.github.com",
		token:      cfg.GitHubToken,
		owner:      cfg.GitHubOwner,
		repo:       cfg.GitHubRepo,
		branch:     cfg.GitHubBranch,
		http:       &http.Client{Timeout: cfg.GitHubAPITimeout},
		apiTimeout: cfg.GitHubAPITimeout,
	}
	client.batcher = NewGitHubBatcher(client, cfg.GitHubBatchMaxItems, cfg.GitHubBatchFlushInterval, cfg.GitHubBatchQueueSize)
	return client
}

func NewGitHubBatcher(github *GitHubClient, maxItems int, flushInterval time.Duration, queueSize int) *GitHubBatcher {
	return &GitHubBatcher{
		github:        github,
		maxItems:      maxItems,
		flushInterval: flushInterval,
		queue:         make(chan GitHubFileUpdate, queueSize),
		done:          make(chan struct{}),
	}
}

func (g *GitHubClient) Start(ctx context.Context) error {
	g.startOnce.Do(func() {
		g.startErr = g.batcher.Start(ctx)
	})
	return g.startErr
}

func (g *GitHubClient) Stop(ctx context.Context) error {
	return g.batcher.Stop(ctx)
}

func (b *GitHubBatcher) Start(parent context.Context) error {
	b.ctx, b.cancel = context.WithCancel(parent)
	go b.loop()
	return nil
}

func (b *GitHubBatcher) Stop(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *GitHubBatcher) Enqueue(ctx context.Context, updates []GitHubFileUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	for _, update := range updates {
		select {
		case b.queue <- update:
		case <-enqueueCtx.Done():
			return fmt.Errorf("enqueue GitHub update: %w", enqueueCtx.Err())
		}
	}
	return nil
}

func (b *GitHubBatcher) loop() {
	defer close(b.done)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	pending := make([]GitHubFileUpdate, 0, b.maxItems)
	flush := func(reason string) {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]GitHubFileUpdate, 0, b.maxItems)
		if err := b.github.CommitFiles(context.Background(), batch); err != nil {
			log.Printf("flush GitHub SBOM batch failed reason=%s files=%d error=%v", reason, len(batch), err)
			return
		}
		log.Printf("flushed GitHub SBOM batch reason=%s files=%d", reason, len(batch))
	}

	for {
		select {
		case update := <-b.queue:
			pending = append(pending, update)
			if len(pending) >= b.maxItems {
				flush("max-items")
			}
		case <-ticker.C:
			flush("interval")
		case <-b.ctx.Done():
			for {
				select {
				case update := <-b.queue:
					pending = append(pending, update)
				default:
					flush("shutdown")
					return
				}
			}
		}
	}
}

func loadConfig() Config {
	cfg := Config{
		Port:                     getenv("PORT", defaultPort),
		ProviderAccessKey:        os.Getenv("PROVIDER_ACCESS_KEY"),
		GitHubToken:              os.Getenv("GITHUB_TOKEN"),
		GitHubOwner:              os.Getenv("GITHUB_OWNER"),
		GitHubRepo:               os.Getenv("GITHUB_REPO"),
		GitHubBranch:             getenv("GITHUB_BRANCH", "main"),
		PathPrefix:               getenv("GITHUB_PATH_PREFIX", defaultPathPrefix),
		SynchronizerURL:          os.Getenv("SYNCHRONIZER_URL"),
		TLSCertFile:              os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:               os.Getenv("TLS_KEY_FILE"),
		GitHubBatchMaxItems:      getenvInt("GITHUB_BATCH_MAX_ITEMS", defaultBatchMaxItems),
		GitHubBatchQueueSize:     getenvInt("GITHUB_BATCH_QUEUE_SIZE", defaultBatchQueueSize),
		GitHubBatchFlushInterval: getenvDuration("GITHUB_BATCH_FLUSH_INTERVAL", defaultBatchFlushInterval),
		GitHubAPITimeout:         getenvDuration("GITHUB_API_TIMEOUT", defaultGitHubAPITimeout),
	}
	missing := []string{}
	if cfg.GitHubToken == "" {
		missing = append(missing, "GITHUB_TOKEN")
	}
	if cfg.GitHubOwner == "" {
		missing = append(missing, "GITHUB_OWNER")
	}
	if cfg.GitHubRepo == "" {
		missing = append(missing, "GITHUB_REPO")
	}
	if cfg.GitHubBatchMaxItems <= 0 {
		log.Fatalf("GITHUB_BATCH_MAX_ITEMS must be greater than 0")
	}
	if cfg.GitHubBatchQueueSize <= 0 {
		log.Fatalf("GITHUB_BATCH_QUEUE_SIZE must be greater than 0")
	}
	if cfg.GitHubBatchFlushInterval <= 0 {
		log.Fatalf("GITHUB_BATCH_FLUSH_INTERVAL must be greater than 0")
	}
	if cfg.GitHubAPITimeout <= 0 {
		log.Fatalf("GITHUB_API_TIMEOUT must be greater than 0")
	}
	if len(missing) > 0 {
		log.Fatalf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return parsed
}

func (s *Server) handleServiceDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	synchronizerURL := s.cfg.SynchronizerURL
	if synchronizerURL == "" {
		scheme := "ws"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "wss"
		}
		synchronizerURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(serviceDiscoveryResponse{
		Version: "v3",
		Response: map[string]string{
			"synchronizer": synchronizerURL,
		},
	}); err != nil {
		log.Printf("write service discovery response: %v", err)
	}
}

func (s *Server) handleSynchronizer(w http.ResponseWriter, r *http.Request) {
	accessKey := r.Header.Get(core.AccessKeyHeader)
	account := r.Header.Get(core.AccountHeader)
	cluster := r.Header.Get(core.ClusterNameHeader)
	remoteAddr := r.RemoteAddr
	log.Printf("synchronizer request received account=%s cluster=%s", account, cluster)
	if s.cfg.ProviderAccessKey != "" && accessKey != s.cfg.ProviderAccessKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if accessKey == "" || account == "" || cluster == "" {
		http.Error(w, "missing synchronizer authentication headers", http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Printf("unable to upgrade connection: %v", err)
		return
	}

	// Do not use r.Context() here. For net/http servers, the request context is
	// canceled when ServeHTTP returns. Because the WebSocket processing continues
	// in a goroutine after this handler returns, using r.Context() would cancel
	// all downstream work, including GitHub API requests.
	ctx, cancel := context.WithCancel(contextFromRequest(context.Background(), r))
	id := ctx.Value(domain.ContextKeyClientIdentifier).(domain.ClientIdentifier)

	go func() {
		defer conn.Close()
		defer cancel()

		log.Printf("synchronizer connected account=%s cluster=%s connection=%s remote=%s", id.Account, id.Cluster, id.ConnectionId, remoteAddr)

		adapter := &GitHubAdapter{cfg: s.cfg, github: s.github}
		syncServer, err := core.NewSynchronizerServer(ctx, []adapters.Adapter{adapter}, conn)
		if err != nil {
			log.Printf("create synchronizer server: %v", err)
			return
		}
		if err := syncServer.Start(ctx); err != nil {
			if isExpectedSynchronizerClose(err) {
				log.Printf("synchronizer connection closed account=%s cluster=%s connection=%s: %v", id.Account, id.Cluster, id.ConnectionId, err)
			} else {
				log.Printf("synchronizer stopped with error account=%s cluster=%s connection=%s: %v", id.Account, id.Cluster, id.ConnectionId, err)
			}
		}

		// Stop expects domain.ContextKeyClientIdentifier to be present in the context.
		// context.Background() would panic inside synchronizer/utils.ClientIdentifierFromContext.
		// Use WithoutCancel(ctx) so cleanup keeps the client identifier values even if
		// the connection context has already been canceled.
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stopCancel()
		if stopErr := syncServer.Stop(stopCtx); stopErr != nil {
			log.Printf("stop synchronizer account=%s cluster=%s connection=%s: %v", id.Account, id.Cluster, id.ConnectionId, stopErr)
		}
	}()
}

func isExpectedSynchronizerClose(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "normal closure") ||
		strings.Contains(msg, "going away") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection reset by peer")
}

func contextFromRequest(parent context.Context, r *http.Request) context.Context {
	id := domain.ClientIdentifier{
		Account:        r.Header.Get(core.AccountHeader),
		Cluster:        r.Header.Get(core.ClusterNameHeader),
		ConnectionId:   newID(),
		ConnectionTime: time.Now(),
		HelmVersion:    r.Header.Get(core.HelmVersionHeader),
		SyncVersion:    r.Header.Get(core.VersionHeader),
		GitVersion:     r.Header.Get(core.GitVersionHeader),
		CloudProvider:  r.Header.Get(core.CloudProviderHeader),
		ClusterUID:     r.Header.Get(core.ClusterUIDHeader),
		ResourceGroup:  r.Header.Get(core.ResourceGroupHeader),
	}
	ctx := context.WithValue(parent, domain.ContextKeyClientIdentifier, id)
	ctx = context.WithValue(ctx, domain.ContextKeyAccessKey, r.Header.Get(core.AccessKeyHeader))
	return ctx
}

func (a *GitHubAdapter) Start(ctx context.Context) error { return nil }

func (a *GitHubAdapter) Stop(ctx context.Context) error { return nil }

func (a *GitHubAdapter) IsRelated(ctx context.Context, id domain.ClientIdentifier) bool { return true }

func (a *GitHubAdapter) RegisterCallbacks(ctx context.Context, callbacks domain.Callbacks) {
	a.callbacks = callbacks
}

func (a *GitHubAdapter) Callbacks(ctx context.Context) (domain.Callbacks, error) {
	return a.callbacks, nil
}

func (a *GitHubAdapter) DeleteObject(ctx context.Context, id domain.KindName) error {
	if isSBOMKind(id.Kind) {
		log.Printf("SBOM deleted in cluster, leaving GitHub file unchanged id=%s", id.String())
	}
	return nil
}

func (a *GitHubAdapter) GetObject(ctx context.Context, id domain.KindName, baseObject []byte) error {
	// This provider is a sink. It does not serve objects back to the cluster.
	return nil
}

func (a *GitHubAdapter) PatchObject(ctx context.Context, id domain.KindName, checksum string, patch []byte) error {
	if !isSBOMKind(id.Kind) {
		return nil
	}
	if a.callbacks.GetObject == nil {
		return errors.New("getObject callback is not registered")
	}
	// Store full SBOM files in GitHub. Ask the cluster side to send the full object instead of applying a patch locally.
	return a.callbacks.GetObject(ctx, id, nil)
}

func (a *GitHubAdapter) PutObject(ctx context.Context, id domain.KindName, checksum string, object []byte) error {
	if !isSBOMKind(id.Kind) {
		return nil
	}
	update, err := a.buildSBOMFileUpdate(ctx, id, checksum, object)
	if err != nil {
		return err
	}
	if err := a.github.QueueFiles(ctx, []GitHubFileUpdate{update}); err != nil {
		return fmt.Errorf("queue GitHub file %s: %w", update.Path, err)
	}
	log.Printf("queued SBOM for GitHub batch path=%s checksum=%s bytes=%d", update.Path, checksum, len(update.Content))
	return nil
}

func (a *GitHubAdapter) VerifyObject(ctx context.Context, id domain.KindName, checksum string) error {
	if !isSBOMKind(id.Kind) {
		return nil
	}
	if a.callbacks.GetObject == nil {
		return errors.New("getObject callback is not registered")
	}
	return a.callbacks.GetObject(ctx, id, nil)
}

func (a *GitHubAdapter) Batch(ctx context.Context, kind domain.Kind, batchType domain.BatchType, items domain.BatchItems) error {
	for _, checksum := range items.NewChecksum {
		id := domain.KindName{Kind: checksum.Kind, Name: checksum.Name, Namespace: checksum.Namespace, ResourceVersion: checksum.ResourceVersion}
		if err := a.VerifyObject(ctx, id, checksum.Checksum); err != nil {
			return err
		}
	}
	for _, patch := range items.PatchObject {
		id := domain.KindName{Kind: patch.Kind, Name: patch.Name, Namespace: patch.Namespace, ResourceVersion: patch.ResourceVersion}
		if err := a.PatchObject(ctx, id, patch.Checksum, []byte(patch.Patch)); err != nil {
			return err
		}
	}

	updates := make([]GitHubFileUpdate, 0, len(items.PutObject))
	for _, put := range items.PutObject {
		id := domain.KindName{Kind: put.Kind, Name: put.Name, Namespace: put.Namespace, ResourceVersion: put.ResourceVersion}
		if !isSBOMKind(id.Kind) {
			continue
		}
		update, err := a.buildSBOMFileUpdate(ctx, id, put.Checksum, []byte(put.Object))
		if err != nil {
			return err
		}
		updates = append(updates, update)
	}
	if err := a.github.QueueFiles(ctx, updates); err != nil {
		return fmt.Errorf("queue GitHub batch files: %w", err)
	}
	if len(updates) > 0 {
		log.Printf("queued SBOM batch for GitHub files=%d batchType=%s", len(updates), batchType)
	}
	return nil
}

func (a *GitHubAdapter) buildSBOMFileUpdate(ctx context.Context, id domain.KindName, checksum string, object []byte) (GitHubFileUpdate, error) {
	payload, err := normalizeObjectPayload(object)
	if err != nil {
		return GitHubFileUpdate{}, fmt.Errorf("normalize object payload: %w", err)
	}
	if len(payload) == 0 {
		return GitHubFileUpdate{}, fmt.Errorf("empty SBOM object for %s/%s", id.Namespace, id.Name)
	}

	var meta SBOMMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return GitHubFileUpdate{}, fmt.Errorf("unmarshal SBOM metadata: %w", err)
	}

	name := firstNonEmpty(id.Name, meta.Metadata.Name, "unknown-sbom")
	namespace := firstNonEmpty(id.Namespace, meta.Metadata.Namespace, "cluster")
	cluster := clusterFromContext(ctx)

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, payload, "", "  "); err == nil {
		payload = pretty.Bytes()
	}

	githubPath := path.Join(a.cfg.PathPrefix, sanitizePathPart(cluster), sanitizePathPart(namespace), sanitizePathPart(name)+".json")
	return GitHubFileUpdate{
		Path:    githubPath,
		Content: payload,
		Source:  fmt.Sprintf("%s checksum=%s", id.String(), checksum),
	}, nil
}

func isSBOMKind(k *domain.Kind) bool {
	if k == nil {
		return false
	}
	group := strings.ToLower(k.Group)
	version := strings.ToLower(k.Version)
	resource := strings.ToLower(k.Resource)
	log.Printf("kind group=%s version=%s resource=%s", group, version, resource)
	if group != "spdx.softwarecomposition.kubescape.io" || version != "v1beta1" {
		return false
	}
	return resource == "sbomsyfts" || resource == "sbomsyftfiltereds"
}

func normalizeObjectPayload(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		return []byte(s), nil
	}
	return trimmed, nil
}

func clusterFromContext(ctx context.Context) string {
	if v := ctx.Value(domain.ContextKeyClientIdentifier); v != nil {
		if id, ok := v.(domain.ClientIdentifier); ok && id.Cluster != "" {
			return id.Cluster
		}
	}
	return "unknown-cluster"
}

func (g *GitHubClient) QueueFiles(ctx context.Context, updates []GitHubFileUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	if err := g.Start(context.Background()); err != nil {
		return err
	}
	return g.batcher.Enqueue(ctx, updates)
}

func (g *GitHubClient) CommitFiles(ctx context.Context, updates []GitHubFileUpdate) error {
	updates = dedupeUpdatesByPath(updates)
	if len(updates) == 0 {
		return nil
	}

	// Updating a Git ref must be serialized. The batcher already has one worker,
	// but this also protects explicit Stop/flush paths and future callers.
	g.mu.Lock()
	defer g.mu.Unlock()

	apiCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.apiTimeout)
	defer cancel()

	blobSHAs := make(map[string]string, len(updates))
	for _, update := range updates {
		sha, err := g.createBlob(apiCtx, update.Content)
		if err != nil {
			return fmt.Errorf("create blob path=%s: %w", update.Path, err)
		}
		blobSHAs[update.Path] = sha
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		headSHA, err := g.getHeadSHA(apiCtx)
		if err != nil {
			return err
		}
		baseTreeSHA, err := g.getCommitTreeSHA(apiCtx, headSHA)
		if err != nil {
			return err
		}
		treeSHA, err := g.createTree(apiCtx, baseTreeSHA, blobSHAs)
		if err != nil {
			return err
		}
		if treeSHA == baseTreeSHA {
			log.Printf("GitHub tree unchanged, skipping batch commit files=%d", len(updates))
			return nil
		}
		commitSHA, err := g.createCommit(apiCtx, batchCommitMessage(updates), treeSHA, headSHA)
		if err != nil {
			return err
		}
		if err := g.updateRef(apiCtx, commitSHA); err != nil {
			lastErr = err
			if isGitHubConflict(err) && attempt < 3 {
				log.Printf("GitHub ref update conflict, retrying attempt=%d files=%d", attempt, len(updates))
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}
			return err
		}
		log.Printf("pushed GitHub SBOM batch files=%d commit=%s", len(updates), commitSHA)
		return nil
	}
	return lastErr
}

func dedupeUpdatesByPath(updates []GitHubFileUpdate) []GitHubFileUpdate {
	if len(updates) <= 1 {
		return updates
	}
	positions := make(map[string]int, len(updates))
	deduped := make([]GitHubFileUpdate, 0, len(updates))
	for _, update := range updates {
		if pos, ok := positions[update.Path]; ok {
			deduped[pos] = update
			continue
		}
		positions[update.Path] = len(deduped)
		deduped = append(deduped, update)
	}
	return deduped
}

func batchCommitMessage(updates []GitHubFileUpdate) string {
	if len(updates) == 1 {
		return fmt.Sprintf("Update Kubescape SBOM %s", updates[0].Path)
	}
	return fmt.Sprintf("Update Kubescape SBOM batch (%d files)", len(updates))
}

func (g *GitHubClient) getHeadSHA(ctx context.Context) (string, error) {
	var out gitRefResponse
	endpoint := fmt.Sprintf("/git/ref/heads/%s", escapeGitHubPath(g.branch))
	if err := g.doJSON(ctx, http.MethodGet, endpoint, nil, &out, http.StatusOK); err != nil {
		return "", err
	}
	if out.Object.SHA == "" {
		return "", fmt.Errorf("GitHub head ref has empty SHA for branch=%s", g.branch)
	}
	return out.Object.SHA, nil
}

func (g *GitHubClient) getCommitTreeSHA(ctx context.Context, commitSHA string) (string, error) {
	var out gitCommitResponse
	endpoint := fmt.Sprintf("/git/commits/%s", url.PathEscape(commitSHA))
	if err := g.doJSON(ctx, http.MethodGet, endpoint, nil, &out, http.StatusOK); err != nil {
		return "", err
	}
	if out.Tree.SHA == "" {
		return "", fmt.Errorf("GitHub commit has empty tree SHA commit=%s", commitSHA)
	}
	return out.Tree.SHA, nil
}

func (g *GitHubClient) createBlob(ctx context.Context, content []byte) (string, error) {
	var out createBlobResponse
	req := createBlobRequest{
		Content:  base64.StdEncoding.EncodeToString(content),
		Encoding: "base64",
	}
	if err := g.doJSON(ctx, http.MethodPost, "/git/blobs", req, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if out.SHA == "" {
		return "", fmt.Errorf("GitHub create blob returned empty SHA")
	}
	return out.SHA, nil
}

func (g *GitHubClient) createTree(ctx context.Context, baseTreeSHA string, blobSHAs map[string]string) (string, error) {
	entries := make([]gitTreeEntry, 0, len(blobSHAs))
	for p, sha := range blobSHAs {
		entries = append(entries, gitTreeEntry{
			Path: p,
			Mode: "100644",
			Type: "blob",
			SHA:  sha,
		})
	}
	var out createTreeResponse
	req := createTreeRequest{
		BaseTree: baseTreeSHA,
		Tree:     entries,
	}
	if err := g.doJSON(ctx, http.MethodPost, "/git/trees", req, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if out.SHA == "" {
		return "", fmt.Errorf("GitHub create tree returned empty SHA")
	}
	return out.SHA, nil
}

func (g *GitHubClient) createCommit(ctx context.Context, message, treeSHA, parentSHA string) (string, error) {
	var out createCommitResponse
	req := createCommitRequest{
		Message: message,
		Tree:    treeSHA,
		Parents: []string{parentSHA},
	}
	if err := g.doJSON(ctx, http.MethodPost, "/git/commits", req, &out, http.StatusCreated); err != nil {
		return "", err
	}
	if out.SHA == "" {
		return "", fmt.Errorf("GitHub create commit returned empty SHA")
	}
	return out.SHA, nil
}

func (g *GitHubClient) updateRef(ctx context.Context, commitSHA string) error {
	req := updateRefRequest{SHA: commitSHA, Force: false}
	endpoint := fmt.Sprintf("/git/refs/heads/%s", escapeGitHubPath(g.branch))
	return g.doJSON(ctx, http.MethodPatch, endpoint, req, nil, http.StatusOK)
}

func (g *GitHubClient) doJSON(ctx context.Context, method, endpoint string, reqBody any, out any, accepted ...int) error {
	var body io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.repoAPIURL(endpoint), body)
	if err != nil {
		return err
	}
	g.setHeaders(req)
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if !statusAccepted(resp.StatusCode, accepted) {
		return &GitHubAPIError{Operation: method + " " + endpoint, StatusCode: resp.StatusCode, Body: truncate(respBody, 500)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode GitHub response %s %s: %w", method, endpoint, err)
	}
	return nil
}

func (g *GitHubClient) repoAPIURL(endpoint string) string {
	return fmt.Sprintf("%s/repos/%s/%s%s", g.baseURL, url.PathEscape(g.owner), url.PathEscape(g.repo), endpoint)
}

func (g *GitHubClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kubescape-github-provider")
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
}

func statusAccepted(status int, accepted []int) bool {
	for _, candidate := range accepted {
		if status == candidate {
			return true
		}
	}
	return false
}

func isGitHubConflict(err error) bool {
	var apiErr *GitHubAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

var badPathChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizePathPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	s = badPathChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		return "unknown"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func escapeGitHubPath(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
