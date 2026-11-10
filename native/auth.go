package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

var graphScopes = []string{"https://graph.microsoft.com/User.Read", "https://graph.microsoft.com/Mail.ReadWrite", "https://graph.microsoft.com/Mail.Send"}
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type localConfig struct {
	ClientID  string `json:"client_id"`
	Tenant    string `json:"tenant"`
	AccountID string `json:"account_id"`
	HTTPToken string `json:"http_token"`
}

func (c localConfig) validate() error {
	if !uuidPattern.MatchString(c.ClientID) {
		return errors.New("client-id must be a Microsoft Entra application ID")
	}
	if c.Tenant != "organizations" && c.Tenant != "common" && !uuidPattern.MatchString(c.Tenant) {
		return errors.New("tenant must be a tenant ID, organizations, or common")
	}
	return nil
}

func stateDirectory() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New("cannot locate the Windows user profile")
	}
	dir := filepath.Join(base, "M365MCP")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := protectDirectory(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func saveProtected(path string, plain []byte) error {
	data, err := protectData(plain)
	if err != nil {
		return errors.New("Windows could not encrypt the local credentials")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func loadProtected(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readBounded(f, 16<<20)
	if err != nil {
		return nil, err
	}
	plain, err := unprotectData(data)
	if err != nil {
		return nil, errors.New("credentials cannot be decrypted by this Windows user; run login again")
	}
	return plain, nil
}

func loadConfig(dir string) (localConfig, error) {
	var config localConfig
	data, err := loadProtected(filepath.Join(dir, "settings.dpapi"))
	if err != nil {
		return config, errors.New("no usable local account; run login first")
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, errors.New("local settings are invalid; run login again")
	}
	if err := config.validate(); err != nil {
		return config, err
	}
	if config.AccountID == "" || len(config.HTTPToken) != 64 {
		return config, errors.New("local account is incomplete; run login again")
	}
	return config, nil
}

type encryptedCache struct{ path string }

func (c encryptedCache) Replace(ctx context.Context, target cache.Unmarshaler, _ cache.ReplaceHints) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := loadProtected(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return target.Unmarshal(data)
}

func (c encryptedCache) Export(ctx context.Context, source cache.Marshaler, _ cache.ExportHints) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := source.Marshal()
	if err != nil {
		return err
	}
	return saveProtected(c.path, data)
}

type localAuth struct {
	app    public.Client
	config localConfig
	mu     sync.Mutex
}

type signInRequiredError struct{}

func (signInRequiredError) Error() string {
	return "Microsoft sign-in is required; stop the server, run login, and regenerate HTTP client configuration if used"
}

func newAuth(dir string, config localConfig) (*localAuth, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	app, err := public.New(config.ClientID,
		public.WithAuthority("https://login.microsoftonline.com/"+config.Tenant),
		public.WithCache(encryptedCache{filepath.Join(dir, "tokens.dpapi")}))
	if err != nil {
		return nil, errors.New("could not initialize Microsoft sign-in")
	}
	return &localAuth{app: app, config: config}, nil
}

func (a *localAuth) login(ctx context.Context, dir string) error {
	result, err := a.app.AcquireTokenInteractive(ctx, graphScopes,
		public.WithRedirectURI("http://localhost"), public.WithOpenURL(openBrowser))
	if err != nil || result.AccessToken == "" || result.Account.HomeAccountID == "" {
		return errors.New("Microsoft sign-in did not complete; verify the app registration and tenant consent")
	}
	a.config.AccountID = result.Account.HomeAccountID
	a.config.HTTPToken = randomToken()
	data, err := json.Marshal(a.config)
	if err != nil {
		return err
	}
	return saveProtected(filepath.Join(dir, "settings.dpapi"), data)
}

func selectAccount(accounts []public.Account, id string) (public.Account, error) {
	for _, account := range accounts {
		if id != "" && account.HomeAccountID == id {
			return account, nil
		}
	}
	return public.Account{}, errors.New("the bound account is unavailable; run login again")
}

func (a *localAuth) token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	accounts, err := a.app.Accounts(ctx)
	if err != nil {
		return "", signInRequiredError{}
	}
	account, err := selectAccount(accounts, a.config.AccountID)
	if err != nil {
		return "", signInRequiredError{}
	}
	result, err := a.app.AcquireTokenSilent(ctx, graphScopes, public.WithSilentAccount(account))
	if err != nil || result.AccessToken == "" || result.Account.HomeAccountID != a.config.AccountID {
		return "", signInRequiredError{}
	}
	return result.AccessToken, nil
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Errorf("secure random source unavailable: %w", err))
	}
	return hex.EncodeToString(value[:])
}
