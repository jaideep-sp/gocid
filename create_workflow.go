package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/spf13/viper"
)

const workflowYAML = `name: Deploy Node.js to EC2

on:
  push:
    branches: [ {{.MainBranch}} ]
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
      
    - name: Use Node.js {{.NodeVersion}}
      uses: actions/setup-node@v3
      with:
        node-version: {{.NodeVersion}}
        cache: 'npm'
        
    - name: Install dependencies
      run: npm ci
        
      
    - name: Build
      run: npm run build --if-present
      
    - name: Configure AWS credentials
      uses: aws-actions/configure-aws-credentials@v1
      with:
        aws-access-key-id: ${{ "{{" }} secrets.AWS_ACCESS_KEY_ID {{ "}}" }}
        aws-secret-access-key: ${{ "{{" }} secrets.AWS_SECRET_ACCESS_KEY {{ "}}" }}
        aws-region: {{.AwsRegion}}
        
    - name: Deploy to EC2
      uses: appleboy/ssh-action@master
      with:
        host: ${{ "{{" }} secrets.EC2_HOST {{ "}}" }}
        username: ${{ "{{" }} secrets.EC2_USER {{ "}}" }}
        key: ${{ "{{" }} secrets.EC2_SSH_KEY {{ "}}" }}
        script: |
          # Install dependencies if they don't exist
          if ! command -v git &> /dev/null; then
            echo "Installing Git..."
            if [ -f /etc/redhat-release ] || [ -f /etc/amazon-release ]; then
              # Amazon Linux, RHEL, CentOS
              sudo yum update -y
              sudo yum install git -y
            else
              # Ubuntu, Debian
              sudo apt update -y
              sudo apt install git -y
            fi
          fi
          
          if ! command -v node &> /dev/null; then
            echo "Installing Node.js {{.NodeVersion}}..."
            if [ -f /etc/redhat-release ] || [ -f /etc/amazon-release ]; then
              # Amazon Linux, RHEL, CentOS
              curl -sL https://rpm.nodesource.com/setup_{{.NodeVersion}} | sudo bash -
              sudo yum install -y nodejs
            else
              # Ubuntu, Debian
              curl -sL https://deb.nodesource.com/setup_{{.NodeVersion}} | sudo -E bash -
              sudo apt install -y nodejs
            fi
          fi
          
          # Ensure lsof is installed for port checking
          if ! command -v lsof &> /dev/null; then
            echo "Installing lsof for port checking..."
            if [ -f /etc/redhat-release ] || [ -f /etc/amazon-release ]; then
              # Amazon Linux, RHEL, CentOS
              sudo yum install -y lsof
            else
              # Ubuntu, Debian
              sudo apt install -y lsof
            fi
          fi
          
          # Print versions for verification
          git --version
          node --version
          npm --version
          
          # Ensure deployment directory exists
          mkdir -p {{.DeploymentPath}}
          cd {{.DeploymentPath}}
          
          # Check if .git directory exists to determine if it's a git repository
          if [ -d ".git" ]; then
            # It's a git repo, try to update it
            echo "Repository exists, updating..."
            git fetch origin
            git reset --hard origin/{{.MainBranch}}
          else
            # If directory is not empty but not a git repo, clean it up
            if [ "$(ls -A .)" ]; then
              echo "Directory not empty. Clearing directory..."
              rm -rf ./* ./.[!.]*
            fi
            # Clone the repository
            echo "Cloning fresh repository..."
            git clone https://github.com/${{ "{{" }} github.repository {{ "}}" }}.git .
          fi
          
          # Install dependencies - using npm install instead of npm ci for projects without package-lock.json
          if [ -f "package-lock.json" ]; then
            echo "Found package-lock.json, using npm ci..."
            npm ci
          else
            echo "No package-lock.json found, using npm install..."
            npm install
          fi
          
          # Build if a build script exists
          npm run build --if-present
          
          # Kill any process running on port 3000
          echo "Checking for processes using port 3000..."
          PORT_PIDS=$(lsof -t -i:3000)
          if [ ! -z "$PORT_PIDS" ]; then
            echo "Found processes using port 3000: $PORT_PIDS"
            echo "Killing processes..."
            for PID in $PORT_PIDS; do
              kill -9 $PID
              echo "Killed process $PID"
            done
          else
            echo "No processes found using port 3000"
          fi
          
          # Wait a moment to ensure port is released
          sleep 2
          
          # Start the application
          echo "Starting application with npm start..."
          nohup npm start > app.log 2>&1 &
          
          # Print status message
          echo "Application started. Check app.log for output."
          echo "Deployment completed successfully"
`

type RepoContentResponse struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

type CreateOrUpdateFileRequest struct {
	Message string `json:"message"`
	Content string `json:"content"`
	SHA     string `json:"sha,omitempty"`
	Branch  string `json:"branch"`
}

func main2() {
	// Setup viper to read from .env file for GitHub token
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	// Try to read the config file
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("Error reading .env file: %v\n", err)
		fmt.Println("Please create a .env file with GITHUB_TOKEN=your_personal_access_token")

		// Create example .env file
		exampleEnv := "GITHUB_TOKEN=your_github_token_here"
		err = os.WriteFile(".env.example", []byte(exampleEnv), 0644)
		if err == nil {
			fmt.Println("Created .env.example as an example")
		}

		os.Exit(1)
	}

	// Get GitHub token from viper
	token := viper.GetString("GITHUB_TOKEN")
	if token == "" || token == "your_github_token_here" {
		fmt.Println("Error: GITHUB_TOKEN not found in .env file or has default value")
		fmt.Println("Please set your GitHub token in the .env file")
		os.Exit(1)
	}

	// Setup viper for deployment config
	viper.SetConfigName("deploy-config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Define defaults
	viper.SetDefault("MainBranch", "main")
	viper.SetDefault("NodeVersion", "16.x")
	viper.SetDefault("AwsRegion", "us-east-1")
	viper.SetDefault("DeploymentPath", "/home/ec2-user/app")
	viper.SetDefault("UseNpm", true)
	viper.SetDefault("StartCommand", "start")
	viper.SetDefault("AppName", "node-app")
	viper.SetDefault("RepoOwner", "")
	viper.SetDefault("RepoName", "")

	// Check if deploy config exists, create example if not
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Config file not found. Creating example config file...")

		exampleConfig := `# GitHub Actions EC2 Deployment Configuration
RepoOwner: yourusername  # Your GitHub username or organization
RepoName: my-nodejs-app  # Name of your GitHub repository
MainBranch: main
NodeVersion: 16.x
AwsRegion: us-east-1
DeploymentPath: /home/ec2-user/app
UseNpm: true  # Set to false if using PM2
StartCommand: start  # Used if UseNpm is true
AppName: node-app  # Used if UseNpm is false (for PM2)
`
		err = os.WriteFile("deploy-config.yaml", []byte(exampleConfig), 0644)
		if err == nil {
			fmt.Println("Created deploy-config.yaml")
			fmt.Println("Please update the values and run the script again.")
		} else {
			fmt.Printf("Error creating config file: %v\n", err)
		}

		os.Exit(1)
	}

	// Get required configuration
	repoOwner := viper.GetString("RepoOwner")
	repoName := viper.GetString("RepoName")

	if repoOwner == "" {
		repoOwner = promptInput("Enter the repository owner (username or organization): ")
		viper.Set("RepoOwner", repoOwner)
	}

	if repoName == "" {
		repoName = promptInput("Enter the repository name: ")
		viper.Set("RepoName", repoName)
	}

	// Generate the workflow content
	workflowContent, err := generateWorkflowContent()
	if err != nil {
		fmt.Printf("Error generating workflow content: %v\n", err)
		os.Exit(1)
	}

	// Create or update the workflow file in the repository
	err = createOrUpdateWorkflow(token, repoOwner, repoName, workflowContent)
	if err != nil {
		fmt.Printf("Error creating/updating workflow: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nWorkflow successfully created/updated in your GitHub repository!")
	fmt.Println("\nIMPORTANT: Add the following secrets to your GitHub repository:")
	fmt.Println("- AWS_ACCESS_KEY_ID")
	fmt.Println("- AWS_SECRET_ACCESS_KEY")
	fmt.Println("- EC2_HOST (your EC2 instance public DNS or IP)")
	fmt.Println("- EC2_USER (usually 'ec2-user' for Amazon Linux or 'ubuntu' for Ubuntu)")
	fmt.Println("- EC2_SSH_KEY (your private SSH key to connect to the EC2 instance)")
}

func promptInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func generateWorkflowContent() (string, error) {
	// Create a template from the workflow YAML
	tmpl, err := template.New("workflow").Parse(workflowYAML)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	// Get values from viper
	data := struct {
		MainBranch     string
		NodeVersion    string
		AwsRegion      string
		DeploymentPath string
		UseNpm         bool
		StartCommand   string
		AppName        string
	}{
		MainBranch:     viper.GetString("MainBranch"),
		NodeVersion:    viper.GetString("NodeVersion"),
		AwsRegion:      viper.GetString("AwsRegion"),
		DeploymentPath: viper.GetString("DeploymentPath"),
		UseNpm:         viper.GetBool("UseNpm"),
		StartCommand:   viper.GetString("StartCommand"),
		AppName:        viper.GetString("AppName"),
	}

	// Render the template with the data
	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}

func createOrUpdateWorkflow(token, owner, repo, content string) error {
	// Define the workflow file path
	path := ".github/workflows/ec2-deploy.yml"

	// Check if file already exists
	fileContent, sha, exists, err := getRepoFileContent(token, owner, repo, path)
	if err != nil && !strings.Contains(err.Error(), "404") {
		return fmt.Errorf("error checking if file exists: %v", err)
	}

	// Create commit message based on whether we're creating or updating
	message := "Add Node.js EC2 deployment workflow"
	if exists {
		message = "Update Node.js EC2 deployment workflow"

		// Check if content is unchanged
		if fileContent == content {
			fmt.Println("Workflow file already exists with the same content. No changes needed.")
			return nil
		}
	} else {
		// If creating, need to ensure directory exists
		dirPath := ".github/workflows"
		_, _, dirExists, _ := getRepoFileContent(token, owner, repo, dirPath)

		if !dirExists {
			// Create .github directory first if needed
			_, _, githubDirExists, _ := getRepoFileContent(token, owner, repo, ".github")
			if !githubDirExists {
				err = createEmptyFile(token, owner, repo, ".github/.gitkeep")
				if err != nil {
					return fmt.Errorf("error creating .github directory: %v", err)
				}
			}

			// Create workflows directory
			err = createEmptyFile(token, owner, repo, dirPath+"/.gitkeep")
			if err != nil {
				return fmt.Errorf("error creating workflows directory: %v", err)
			}
		}
	}

	// Encode content to base64
	encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

	// Create request body
	requestBody := CreateOrUpdateFileRequest{
		Message: message,
		Content: encodedContent,
		Branch:  viper.GetString("MainBranch"),
	}

	// Add SHA if file exists (for update)
	if exists {
		requestBody.SHA = sha
	}

	// Marshal request body to JSON
	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(requestJSON))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Add headers
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response from GitHub API: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

func getRepoFileContent(token, owner, repo, path string) (string, string, bool, error) {
	// Create HTTP request
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", false, fmt.Errorf("error creating request: %v", err)
	}

	// Add headers
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check if file exists
	if resp.StatusCode == http.StatusNotFound {
		return "", "", false, nil
	}

	// Check for other errors
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", false, fmt.Errorf("error response from GitHub API: %s - %s", resp.Status, string(bodyBytes))
	}

	// Parse response
	var contentResp RepoContentResponse
	err = json.NewDecoder(resp.Body).Decode(&contentResp)
	if err != nil {
		return "", "", true, fmt.Errorf("error decoding response: %v", err)
	}

	// Decode content from base64
	decodedContent, err := base64.StdEncoding.DecodeString(contentResp.Content)
	if err != nil {
		return "", "", true, fmt.Errorf("error decoding content: %v", err)
	}

	return string(decodedContent), contentResp.SHA, true, nil
}

func createEmptyFile(token, owner, repo, path string) error {
	// Create an empty file to ensure directory exists
	emptyContent := ""
	encodedContent := base64.StdEncoding.EncodeToString([]byte(emptyContent))

	requestBody := CreateOrUpdateFileRequest{
		Message: "Create directory structure for GitHub Actions",
		Content: encodedContent,
		Branch:  viper.GetString("MainBranch"),
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %v", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(requestJSON))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response from GitHub API: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}
