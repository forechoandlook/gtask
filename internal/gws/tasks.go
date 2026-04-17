package gws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// The following variables are set at build time using -ldflags in CI
var (
	BuiltinClientID     string
	BuiltinClientSecret string
)

const (
	apiBaseURL = "https://tasks.googleapis.com/tasks/v1"
)

type Client struct {
	httpClient *http.Client
}

type TaskList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type TaskListListResponse struct {
	Items []TaskList `json:"items"`
}

type RemoteTask struct {
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Notes  string `json:"notes,omitempty"`
	Due    string `json:"due,omitempty"`
	Status string `json:"status,omitempty"`
}

type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func NewClient(ctx context.Context) (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}

	gtaskDir := filepath.Join(home, ".gtask")
	credsPath := filepath.Join(gtaskDir, "client_secret.json")
	tokenPath := filepath.Join(gtaskDir, "token.json")

	if err := os.MkdirAll(gtaskDir, 0755); err != nil {
		return nil, fmt.Errorf("create gtask dir: %w", err)
	}

	var oauthConfig *oauth2.Config

	// 1. Try loading user-provided client_secret.json from disk
	if data, err := os.ReadFile(credsPath); err == nil {
		var root struct {
			Installed Credentials `json:"installed"`
		}
		if err := json.Unmarshal(data, &root); err == nil && root.Installed.ClientID != "" {
			oauthConfig = &oauth2.Config{
				ClientID:     root.Installed.ClientID,
				ClientSecret: root.Installed.ClientSecret,
				Endpoint:     google.Endpoint,
				Scopes:       []string{"https://www.googleapis.com/auth/tasks"},
				RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
			}
		}
	}

	// 2. Fallback to builtin credentials
	if oauthConfig == nil && BuiltinClientID != "" && BuiltinClientSecret != "" {
		oauthConfig = &oauth2.Config{
			ClientID:     BuiltinClientID,
			ClientSecret: BuiltinClientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"https://www.googleapis.com/auth/tasks"},
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		}
		// Save builtin for future runs
		creds := Credentials{ClientID: BuiltinClientID, ClientSecret: BuiltinClientSecret}
		saveJSON(credsPath, map[string]any{"installed": creds})
	}

	// 3. Last resort: Ask user
	if oauthConfig == nil {
		fmt.Println("Google OAuth credentials not found.")
		fmt.Print("Client ID: ")
		var cid string
		fmt.Scan(&cid)
		fmt.Print("Client Secret: ")
		var sec string
		fmt.Scan(&sec)

		if cid == "" || sec == "" {
			return nil, fmt.Errorf("client ID and Secret are required")
		}

		creds := Credentials{ClientID: cid, ClientSecret: sec}
		saveJSON(credsPath, map[string]any{"installed": creds})
		oauthConfig = &oauth2.Config{
			ClientID:     cid,
			ClientSecret: sec,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"https://www.googleapis.com/auth/tasks"},
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		}
	}

	// Token handling
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		token, err = getTokenFromWeb(oauthConfig)
		if err != nil {
			return nil, err
		}
		saveJSON(tokenPath, token)
	}

	return &Client{
		httpClient: oauthConfig.Client(ctx, token),
	}, nil
}

func (c *Client) ListTaskLists(ctx context.Context) ([]TaskList, error) {
	url := fmt.Sprintf("%s/users/@me/lists", apiBaseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	var res TaskListListResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Items, nil
}

func (c *Client) InsertTask(ctx context.Context, taskListID string, payload map[string]any) (RemoteTask, error) {
	url := fmt.Sprintf("%s/lists/%s/tasks", apiBaseURL, taskListID)
	data, _ := json.Marshal(payload)
	
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return RemoteTask{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return RemoteTask{}, fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	var task RemoteTask
	json.NewDecoder(resp.Body).Decode(&task)
	return task, nil
}

func (c *Client) UpdateTask(ctx context.Context, taskListID, taskID string, payload map[string]any) (RemoteTask, error) {
	url := fmt.Sprintf("%s/lists/%s/tasks/%s", apiBaseURL, taskListID, taskID)
	data, _ := json.Marshal(payload)
	
	req, _ := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return RemoteTask{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return RemoteTask{}, fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	var task RemoteTask
	json.NewDecoder(resp.Body).Decode(&task)
	return task, nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)
	var authCode string
	fmt.Print("Authorization Code: ")
	fmt.Scan(&authCode)
	return config.Exchange(context.TODO(), authCode)
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveJSON(path string, v any) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(v)
}
