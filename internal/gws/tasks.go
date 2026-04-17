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

func NewClient(ctx context.Context) (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}

	gtaskDir := filepath.Join(home, ".gtask")
	tokenPath := filepath.Join(gtaskDir, "token.json")
	
	// Try loading client secret from gtask dir first, then fallback to gws dir
	secretPaths := []string{
		filepath.Join(gtaskDir, "client_secret.json"),
		filepath.Join(home, ".config", "gws", "client_secret.json"),
	}

	var secretData []byte
	for _, p := range secretPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			secretData = data
			break
		}
	}

	if len(secretData) == 0 {
		return nil, fmt.Errorf("client_secret.json not found in ~/.gtask or ~/.config/gws/. Please place your Google OAuth credentials file there.")
	}

	config, err := google.ConfigFromJSON(secretData, tasks.TasksScope)
	if err != nil {
		return nil, fmt.Errorf("parse client secret: %w", err)
	}
	// Important for CLI: set RedirectURL to OOB if needed, 
	// though google.ConfigFromJSON usually picks it up from the JSON if present.
	if len(config.RedirectURIs) > 0 && config.RedirectURIs[0] == "urn:ietf:wg:oauth:2.0:oob" {
		config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	}

	// 1. Try loading local gtask token
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		// 2. Fallback: try migrating from gws if exists
		gwsTokenPath := filepath.Join(home, ".config", "gws", "token_cache.json")
		token, err = tryMigrateGwsToken(gwsTokenPath)
		if err != nil {
			// 3. Last resort: Interactive Auth
			token, err = getTokenFromWeb(config)
			if err != nil {
				return nil, err
			}
			saveToken(tokenPath, token)
		} else {
			// Save the migrated token to our own path
			saveToken(tokenPath, token)
		}
	}

	httpClient := config.Client(ctx, token)
	srv, err := tasks.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create tasks service: %w", err)
	}

	return &Client{service: srv}, nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
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

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func tryMigrateGwsToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err == nil {
		return &token, nil
	}
	// Try map fallback
	var tokenMap map[string]*oauth2.Token
	if err := json.Unmarshal(data, &tokenMap); err == nil {
		for _, v := range tokenMap {
			return v, nil
		}
	}
	return nil, fmt.Errorf("could not parse gws token")
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
