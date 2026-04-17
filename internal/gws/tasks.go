package gws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"
)

// The following variables can be set at build time using -ldflags
var (
	BuiltinClientID     string
	BuiltinClientSecret string
)

type Client struct {
	service *tasks.Service
}

type TaskList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type RemoteTask struct {
	ID string `json:"id"`
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
	credsPath := filepath.Join(gtaskDir, "credentials.json")
	tokenPath := filepath.Join(gtaskDir, "token.json")

	if err := os.MkdirAll(gtaskDir, 0755); err != nil {
		return nil, fmt.Errorf("create gtask dir: %w", err)
	}

	var oauthConfig *oauth2.Config

	// 1. Try loading user-provided credentials from disk
	if conf, err := loadConfig(credsPath); err == nil {
		oauthConfig = conf
	} else if BuiltinClientID != "" && BuiltinClientSecret != "" {
		// 2. Try using builtin credentials injected during build
		oauthConfig = &oauth2.Config{
			ClientID:     BuiltinClientID,
			ClientSecret: BuiltinClientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       []string{tasks.TasksScope},
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		}
	} else {
		// 3. Last resort: Ask user for them
		fmt.Println("Google OAuth credentials not found.")
		fmt.Println("Please provide your Google Client ID and Client Secret.")
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
		saveJSON(credsPath, creds)
		oauthConfig = &oauth2.Config{
			ClientID:     cid,
			ClientSecret: sec,
			Endpoint:     google.Endpoint,
			Scopes:       []string{tasks.TasksScope},
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		}
	}

	// Load or Ask for User Token
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		token, err = getTokenFromWeb(oauthConfig)
		if err != nil {
			return nil, err
		}
		saveJSON(tokenPath, token)
	}

	httpClient := oauthConfig.Client(ctx, token)
	srv, err := tasks.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create tasks service: %w", err)
	}

	return &Client{service: srv}, nil
}

func loadConfig(path string) (*oauth2.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var creds Credentials
	if err := json.NewDecoder(f).Decode(&creds); err != nil {
		return nil, err
	}
	return &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{tasks.TasksScope},
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	}, nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)
	var authCode string
	fmt.Print("Authorization Code: ")
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}
	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %v", err)
	}
	return tok, nil
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

func (c *Client) ListTaskLists(ctx context.Context) ([]TaskList, error) {
	res, err := c.service.Tasklists.List().Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var lists []TaskList
	for _, item := range res.Items {
		lists = append(lists, TaskList{ID: item.Id, Title: item.Title})
	}
	return lists, nil
}

func (c *Client) InsertTask(ctx context.Context, taskListID string, payload map[string]any) (RemoteTask, error) {
	task := &tasks.Task{}
	if v, ok := payload["title"].(string); ok {
		task.Title = v
	}
	if v, ok := payload["notes"].(string); ok {
		task.Notes = v
	}
	if v, ok := payload["due"].(string); ok {
		task.Due = v
	}
	if v, ok := payload["status"].(string); ok {
		task.Status = v
	}
	res, err := c.service.Tasks.Insert(taskListID, task).Context(ctx).Do()
	if err != nil {
		return RemoteTask{}, err
	}
	return RemoteTask{ID: res.Id}, nil
}

func (c *Client) UpdateTask(ctx context.Context, taskListID, taskID string, payload map[string]any) (RemoteTask, error) {
	task := &tasks.Task{}
	if v, ok := payload["title"].(string); ok {
		task.Title = v
	}
	if v, ok := payload["notes"].(string); ok {
		task.Notes = v
	}
	if v, ok := payload["due"].(string); ok {
		task.Due = v
	}
	if v, ok := payload["status"].(string); ok {
		task.Status = v
	}
	res, err := c.service.Tasks.Patch(taskListID, taskID, task).Context(ctx).Do()
	if err != nil {
		return RemoteTask{}, err
	}
	return RemoteTask{ID: res.Id}, nil
}
