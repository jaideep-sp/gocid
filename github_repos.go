package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Repository represents the GitHub repository structure
type Repository struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
}

func main1() {
	// Setup viper to read from .env file
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	// Try to read the config file
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		fmt.Println("Please create a .env file in the same directory with GITHUB_TOKEN=your_token")

		// Create example .env file
		exampleEnv := "GITHUB_TOKEN=your_github_token_here"
		err = os.WriteFile(".env.example", []byte(exampleEnv), 0644)
		if err == nil {
			fmt.Printf("Created %s as an example\n", filepath.Join(".", ".env.example"))
		}

		os.Exit(1)
	}

	// Get token from viper
	token := viper.GetString("GITHUB_TOKEN")
	if token == "" || token == "your_github_token_here" {
		fmt.Println("Error: GITHUB_TOKEN not found in .env file or has default value")
		fmt.Println("Please set your GitHub token in the .env file")
		os.Exit(1)
	}

	// Create a new HTTP client and request
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.github.com/user/repos?per_page=100", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		os.Exit(1)
	}

	// Add authorization header
	req.Header.Add("Authorization", "token "+token)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("API request failed with status code %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON response
	var repos []Repository
	err = json.Unmarshal(body, &repos)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	// Print repositories
	fmt.Printf("Found %d repositories:\n\n", len(repos))
	for i, repo := range repos {
		visibility := "public"
		if repo.Private {
			visibility = "private"
		}

		fmt.Printf("%d. %s (%s)\n", i+1, repo.FullName, visibility)
		fmt.Printf("   URL: %s\n", repo.HTMLURL)
		if repo.Description != "" {
			fmt.Printf("   Description: %s\n", repo.Description)
		}
		fmt.Println()
	}
}
