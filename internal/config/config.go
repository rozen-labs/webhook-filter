package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rozen-labs/webhook-filter/internal/expression"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Logging  LoggingConfig  `yaml:"logging"`
	Security SecurityConfig `yaml:"security"`
	Routes   []RouteConfig  `yaml:"routes"`
}

type ServerConfig struct {
	ListenAddr            string `yaml:"listen_addr"`
	RequestTimeoutSeconds int    `yaml:"request_timeout_seconds"`
	MaxBodyBytes          int64  `yaml:"max_body_bytes"`
}

type LoggingConfig struct {
	Level   string `yaml:"level"`
	Format  string `yaml:"format"`
	LogBody bool   `yaml:"log_body"`
}

type SecurityConfig struct {
	DefaultFilteredStatusCode int    `yaml:"default_filtered_status_code"`
	DefaultFilteredBody       string `yaml:"default_filtered_body"`
}

type RouteConfig struct {
	Name       string          `yaml:"name"`
	Match      MatchConfig     `yaml:"match"`
	Auth       AuthConfig      `yaml:"auth"`
	Conditions ConditionConfig `yaml:"conditions"`
	Config     map[string]any  `yaml:"config"`
	Forward    ForwardConfig   `yaml:"forward"`
	Response   ResponseConfig  `yaml:"response"`
}

type MatchConfig struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
}

type ConditionConfig struct {
	Expression string `yaml:"expression"`
}

type AuthConfig struct {
	Type            string       `yaml:"type,omitempty"`
	Header          string       `yaml:"header,omitempty"`
	SecretEnv       string       `yaml:"secret_env,omitempty"`
	SignatureHeader string       `yaml:"signature_header,omitempty"`
	SignaturePrefix string       `yaml:"signature_prefix,omitempty"`
	All             []AuthConfig `yaml:"all,omitempty"`
	Any             []AuthConfig `yaml:"any,omitempty"`
}

type ForwardConfig struct {
	URL             string            `yaml:"url"`
	Method          string            `yaml:"method"`
	PreserveBody    *bool             `yaml:"preserve_body"`
	PreserveHeaders []string          `yaml:"preserve_headers"`
	AddHeaders      map[string]string `yaml:"add_headers"`
	TimeoutSeconds  int               `yaml:"timeout_seconds"`
}

type ResponseConfig struct {
	OnFiltered       ResponseMessage    `yaml:"on_filtered"`
	OnForwardSuccess ForwardSuccessMode `yaml:"on_forward_success"`
	OnForwardError   ResponseMessage    `yaml:"on_forward_error"`
}

type ForwardSuccessMode struct {
	Mode       string            `yaml:"mode"`
	StatusCode int               `yaml:"status_code,omitempty"`
	Body       string            `yaml:"body,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
}

type ResponseMessage struct {
	StatusCode int    `yaml:"status_code"`
	Body       string `yaml:"body"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.RequestTimeoutSeconds <= 0 {
		cfg.Server.RequestTimeoutSeconds = 10
	}
	if cfg.Server.MaxBodyBytes <= 0 {
		cfg.Server.MaxBodyBytes = 1048576
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Security.DefaultFilteredStatusCode == 0 {
		cfg.Security.DefaultFilteredStatusCode = 202
	}
	if cfg.Security.DefaultFilteredBody == "" {
		cfg.Security.DefaultFilteredBody = "ignored"
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].Forward.TimeoutSeconds <= 0 {
			cfg.Routes[i].Forward.TimeoutSeconds = 5
		}
	}
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.ListenAddr) == "" {
		return errors.New("server.listen_addr is required")
	}
	if c.Server.RequestTimeoutSeconds <= 0 {
		return errors.New("server.request_timeout_seconds must be > 0")
	}
	if c.Server.MaxBodyBytes <= 0 {
		return errors.New("server.max_body_bytes must be > 0")
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Level != "debug" && c.Logging.Level != "info" && c.Logging.Level != "warn" && c.Logging.Level != "error" {
		return fmt.Errorf("logging.level must be debug, info, warn, or error")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return fmt.Errorf("logging.format must be json or text")
	}
	if c.Security.DefaultFilteredStatusCode == 0 {
		c.Security.DefaultFilteredStatusCode = 202
	}
	if c.Security.DefaultFilteredBody == "" {
		c.Security.DefaultFilteredBody = "ignored"
	}
	if len(c.Routes) == 0 {
		return errors.New("at least one route is required")
	}
	seen := map[string]struct{}{}
	for i := range c.Routes {
		r := &c.Routes[i]
		r.Name = strings.TrimSpace(r.Name)
		if r.Name == "" {
			return fmt.Errorf("routes[%d].name is required", i)
		}
		if _, ok := seen[r.Name]; ok {
			return fmt.Errorf("duplicate route name %q", r.Name)
		}
		seen[r.Name] = struct{}{}
		if strings.TrimSpace(r.Match.Path) == "" {
			return fmt.Errorf("routes[%s].match.path is required", r.Name)
		}
		if !validHTTPMethod(r.Match.Method) {
			return fmt.Errorf("routes[%s].match.method is invalid: %q", r.Name, r.Match.Method)
		}
		if strings.TrimSpace(r.Conditions.Expression) == "" {
			return fmt.Errorf("routes[%s].conditions.expression is required", r.Name)
		}
		if err := expression.Validate(r.Conditions.Expression); err != nil {
			return fmt.Errorf("routes[%s].conditions.expression: %w", r.Name, err)
		}
		if err := validateAuth(r.Auth, r.Name); err != nil {
			return err
		}
		if _, err := url.ParseRequestURI(r.Forward.URL); err != nil {
			return fmt.Errorf("routes[%s].forward.url invalid: %w", r.Name, err)
		}
		if r.Forward.TimeoutSeconds <= 0 {
			return fmt.Errorf("routes[%s].forward.timeout_seconds must be > 0", r.Name)
		}
		if r.Forward.PreserveBody == nil {
			v := true
			r.Forward.PreserveBody = &v
		}
		if !validHTTPMethod(r.Forward.Method) {
			return fmt.Errorf("routes[%s].forward.method is invalid: %q", r.Name, r.Forward.Method)
		}
		if r.Response.OnForwardSuccess.Mode == "" {
			r.Response.OnForwardSuccess.Mode = "proxy"
		}
		if r.Response.OnForwardSuccess.Mode != "proxy" && r.Response.OnForwardSuccess.Mode != "static" {
			return fmt.Errorf("routes[%s].response.on_forward_success.mode must be proxy or static", r.Name)
		}
		if r.Response.OnFiltered.StatusCode == 0 {
			r.Response.OnFiltered.StatusCode = c.Security.DefaultFilteredStatusCode
		}
		if r.Response.OnFiltered.Body == "" {
			r.Response.OnFiltered.Body = c.Security.DefaultFilteredBody
		}
		if r.Response.OnForwardError.StatusCode == 0 {
			r.Response.OnForwardError.StatusCode = 502
		}
		if r.Response.OnForwardError.Body == "" {
			r.Response.OnForwardError.Body = "forward failed"
		}
		if r.Forward.PreserveHeaders == nil {
			r.Forward.PreserveHeaders = []string{}
		}
		if r.Forward.AddHeaders == nil {
			r.Forward.AddHeaders = map[string]string{}
		}
		if r.Config == nil {
			r.Config = map[string]any{}
		}
	}
	return nil
}

func validHTTPMethod(method string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return false
	}
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT":
		return true
	default:
		return false
	}
}

func validateAuth(a AuthConfig, routeName string) error {
	if len(a.All) > 0 && len(a.Any) > 0 {
		return fmt.Errorf("routes[%s].auth cannot set both all and any", routeName)
	}
	if len(a.All) > 0 {
		for i := range a.All {
			if err := validateAuth(a.All[i], routeName); err != nil {
				return err
			}
		}
		return nil
	}
	if len(a.Any) > 0 {
		for i := range a.Any {
			if err := validateAuth(a.Any[i], routeName); err != nil {
				return err
			}
		}
		return nil
	}

	if a.Type == "" {
		a.Type = "none"
	}
	switch a.Type {
	case "none":
		return nil
	case "header_secret":
		if a.Header == "" {
			return fmt.Errorf("routes[%s].auth.header_secret.header is required", routeName)
		}
		if a.SecretEnv == "" {
			return fmt.Errorf("routes[%s].auth.header_secret.secret_env is required", routeName)
		}
	case "bearer":
		if a.SecretEnv == "" {
			return fmt.Errorf("routes[%s].auth.bearer.secret_env is required", routeName)
		}
	case "github_signature":
		if a.SecretEnv == "" {
			return fmt.Errorf("routes[%s].auth.github_signature.secret_env is required", routeName)
		}
	case "hmac_sha256":
		if a.SecretEnv == "" || a.SignatureHeader == "" || a.SignaturePrefix == "" {
			return fmt.Errorf("routes[%s].auth.hmac_sha256.secret_env, signature_header, and signature_prefix are required", routeName)
		}
	default:
		return fmt.Errorf("routes[%s].auth type %q is unsupported", routeName, a.Type)
	}
	if _, ok := os.LookupEnv(a.SecretEnv); !ok {
		return fmt.Errorf("routes[%s] secret env %q is missing", routeName, a.SecretEnv)
	}
	return nil
}

func (c *Config) RouteNames() []string {
	names := make([]string, 0, len(c.Routes))
	for _, r := range c.Routes {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names
}

func TimeoutDuration(seconds int) time.Duration { return time.Duration(seconds) * time.Second }
