// package main

// import (
// 	"context"
// 	"encoding/base64"
// 	"fmt"
// 	"io/ioutil"
// 	"log"
// 	"os"
// 	"time"

// 	"github.com/aws/aws-sdk-go-v2/aws"
// 	"github.com/aws/aws-sdk-go-v2/credentials"
// 	"github.com/aws/aws-sdk-go-v2/service/ec2"
// 	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
// 	"github.com/aws/aws-sdk-go-v2/service/iam"
// 	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
// 	"github.com/google/go-github/v45/github"
// 	"github.com/joho/godotenv"
// 	"golang.org/x/oauth2"
// )

// // Configuration holds all the parameters needed for setup
// type Configuration struct {
// 	// AWS Configuration
// 	AwsRegion       string
// 	AwsAccessKey    string
// 	AwsSecretKey    string
// 	InstanceType    string
// 	AmiID           string
// 	KeyName         string
// 	SecurityGroup   string
// 	InstanceName    string
// 	IamRoleName     string

// 	// GitHub Configuration
// 	GithubToken     string
// 	GithubOwner     string
// 	GithubRepo      string

// 	// Node.js Configuration
// 	NodeVersion     string
// 	MainBranch      string
// 	DeploymentPath  string
// 	AppName         string
// 	StartCommand    string
// }

// // loadEnvConfig loads configuration from environment file
// func loadEnvConfig(envFile string) (Configuration, error) {
// 	var config Configuration

// 	// Load .env file if it exists
// 	if err := godotenv.Load(envFile); err != nil {
// 		return config, fmt.Errorf("error loading env file: %v", err)
// 	}

// 	// AWS Configuration
// 	config.AwsRegion = getEnvWithDefault("AWS_REGION", "us-east-1")
// 	config.AwsAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
// 	config.AwsSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
// 	config.InstanceType = getEnvWithDefault("INSTANCE_TYPE", "t2.micro")
// 	config.AmiID = getEnvWithDefault("AMI_ID", "ami-0c55b159cbfafe1f0") // Amazon Linux 2
// 	config.KeyName = os.Getenv("KEY_NAME")
// 	config.SecurityGroup = getEnvWithDefault("SECURITY_GROUP", "nodejs-app-sg")
// 	config.InstanceName = getEnvWithDefault("INSTANCE_NAME", "nodejs-app-server")
// 	config.IamRoleName = getEnvWithDefault("IAM_ROLE_NAME", "EC2NodejsDeploymentRole")

// 	// GitHub Configuration
// 	config.GithubToken = os.Getenv("GITHUB_TOKEN")
// 	config.GithubOwner = os.Getenv("GITHUB_OWNER")
// 	config.GithubRepo = os.Getenv("GITHUB_REPO")

// 	// Node.js Configuration
// 	config.NodeVersion = getEnvWithDefault("NODE_VERSION", "16")
// 	config.MainBranch = getEnvWithDefault("MAIN_BRANCH", "main")
// 	config.DeploymentPath = getEnvWithDefault("DEPLOYMENT_PATH", "/var/www/nodejs-app")
// 	config.AppName = getEnvWithDefault("APP_NAME", "nodejs-app")
// 	config.StartCommand = getEnvWithDefault("START_COMMAND", "start")

// 	// Validate required fields
// 	if config.AwsAccessKey == "" || config.AwsSecretKey == "" {
// 		return config, fmt.Errorf("AWS credentials are required")
// 	}
// 	if config.KeyName == "" {
// 		return config, fmt.Errorf("KEY_NAME is required (your EC2 key pair name)")
// 	}
// 	if config.GithubToken == "" || config.GithubOwner == "" || config.GithubRepo == "" {
// 		return config, fmt.Errorf("GitHub configuration is required")
// 	}

// 	return config, nil
// }

// // getEnvWithDefault gets an environment variable with a default value
// func getEnvWithDefault(key, defaultValue string) string {
// 	value := os.Getenv(key)
// 	if value == "" {
// 		return defaultValue
// 	}
// 	return value
// }

// // createAwsClient creates an AWS config with specific credentials
// func createAwsClient(config Configuration) (aws.Config, error) {
// 	staticProvider := credentials.NewStaticCredentialsProvider(config.AwsAccessKey, config.AwsSecretKey, "")

// 	cfg, err := config.LoadDefaultConfig(
// 		context.TODO(),
// 		config.WithRegion(config.AwsRegion),
// 		config.WithCredentialsProvider(staticProvider),
// 	)

// 	if err != nil {
// 		return aws.Config{}, fmt.Errorf("failed to load AWS config: %v", err)
// 	}

// 	return cfg, nil
// }

// // createSecurityGroup creates a security group for the EC2 instance
// func createSecurityGroup(ctx context.Context, client *ec2.Client, config Configuration) (string, error) {
// 	// Check if security group already exists
// 	describeInput := &ec2.DescribeSecurityGroupsInput{
// 		Filters: []types.Filter{
// 			{
// 				Name:   aws.String("group-name"),
// 				Values: []string{config.SecurityGroup},
// 			},
// 		},
// 	}

// 	describeResult, err := client.DescribeSecurityGroups(ctx, describeInput)
// 	if err == nil && len(describeResult.SecurityGroups) > 0 {
// 		sgID := *describeResult.SecurityGroups[0].GroupId
// 		log.Printf("Security group already exists: %s (%s)", config.SecurityGroup, sgID)
// 		return sgID, nil
// 	}

// 	// Create new security group
// 	createInput := &ec2.CreateSecurityGroupInput{
// 		GroupName:   aws.String(config.SecurityGroup),
// 		Description: aws.String("Security group for Node.js application"),
// 	}

// 	createResult, err := client.CreateSecurityGroup(ctx, createInput)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create security group: %v", err)
// 	}
// 	sgID := *createResult.GroupId

// 	// Add inbound rules
// 	_, err = client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
// 		GroupId: aws.String(sgID),
// 		IpPermissions: []types.IpPermission{
// 			// SSH (port 22)
// 			{
// 				IpProtocol: aws.String("tcp"),
// 				FromPort:   aws.Int32(22),
// 				ToPort:     aws.Int32(22),
// 				IpRanges: []types.IpRange{
// 					{
// 						CidrIp:      aws.String("0.0.0.0/0"),
// 						Description: aws.String("Allow SSH access"),
// 					},
// 				},
// 			},
// 			// HTTP (port 80)
// 			{
// 				IpProtocol: aws.String("tcp"),
// 				FromPort:   aws.Int32(80),
// 				ToPort:     aws.Int32(80),
// 				IpRanges: []types.IpRange{
// 					{
// 						CidrIp:      aws.String("0.0.0.0/0"),
// 						Description: aws.String("Allow HTTP access"),
// 					},
// 				},
// 			},
// 			// HTTPS (port 443)
// 			{
// 				IpProtocol: aws.String("tcp"),
// 				FromPort:   aws.Int32(443),
// 				ToPort:     aws.Int32(443),
// 				IpRanges: []types.IpRange{
// 					{
// 						CidrIp:      aws.String("0.0.0.0/0"),
// 						Description: aws.String("Allow HTTPS access"),
// 					},
// 				},
// 			},
// 			// Application port (port 3000)
// 			{
// 				IpProtocol: aws.String("tcp"),
// 				FromPort:   aws.Int32(3000),
// 				ToPort:     aws.Int32(3000),
// 				IpRanges: []types.IpRange{
// 					{
// 						CidrIp:      aws.String("0.0.0.0/0"),
// 						Description: aws.String("Allow application port"),
// 					},
// 				},
// 			},
// 		},
// 	})

// 	if err != nil {
// 		return "", fmt.Errorf("failed to authorize security group ingress: %v", err)
// 	}

// 	log.Printf("Created security group: %s (%s)", config.SecurityGroup, sgID)
// 	return sgID, nil
// }

// // createIAMRole creates an IAM role for the EC2 instance
// func createIAMRole(ctx context.Context, config Configuration) (string, error) {
// 	cfg, err := createAwsClient(config)
// 	if err != nil {
// 		return "", err
// 	}

// 	client := iam.NewFromConfig(cfg)

// 	// Check if role already exists
// 	_, err = client.GetRole(ctx, &iam.GetRoleInput{
// 		RoleName: aws.String(config.IamRoleName),
// 	})

// 	if err == nil {
// 		log.Printf("IAM role already exists: %s", config.IamRoleName)
// 		return config.IamRoleName, nil
// 	}

// 	// Create the IAM role
// 	trustPolicy := `{
// 		"Version": "2012-10-17",
// 		"Statement": [
// 			{
// 				"Effect": "Allow",
// 				"Principal": {
// 					"Service": "ec2.amazonaws.com"
// 				},
// 				"Action": "sts:AssumeRole"
// 			}
// 		]
// 	}`

// 	createRoleResult, err := client.CreateRole(ctx, &iam.CreateRoleInput{
// 		RoleName:                 aws.String(config.IamRoleName),
// 		AssumeRolePolicyDocument: aws.String(trustPolicy),
// 		Description:              aws.String("Role for EC2 Node.js deployment"),
// 	})

// 	if err != nil {
// 		return "", fmt.Errorf("failed to create IAM role: %v", err)
// 	}

// 	// Attach policies
// 	_, err = client.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
// 		RoleName:  aws.String(config.IamRoleName),
// 		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess"),
// 	})

// 	if err != nil {
// 		return "", fmt.Errorf("failed to attach policy to role: %v", err)
// 	}

// 	// Create instance profile
// 	_, err = client.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
// 		InstanceProfileName: aws.String(config.IamRoleName),
// 	})

// 	if err != nil {
// 		// Check if the error is because it already exists
// 		var existsErr *iamtypes.EntityAlreadyExistsException
// 		if !aws.IsErrorCode(err, "EntityAlreadyExists") {
// 			return "", fmt.Errorf("failed to create instance profile: %v", err)
// 		}
// 	}

// 	// Add role to instance profile
// 	_, err = client.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
// 		InstanceProfileName: aws.String(config.IamRoleName),
// 		RoleName:            aws.String(config.IamRoleName),
// 	})

// 	if err != nil {
// 		var existsErr *iamtypes.LimitExceededException
// 		if !aws.IsErrorCode(err, "LimitExceeded") {
// 			return "", fmt.Errorf("failed to add role to instance profile: %v", err)
// 		}
// 	}

// 	// Wait for role to propagate
// 	log.Println("Waiting for IAM role to propagate...")
// 	time.Sleep(10 * time.Second)

// 	return *createRoleResult.Role.RoleName, nil
// }

// // launchEC2Instance launches an EC2 instance
// func launchEC2Instance(ctx context.Context, config Configuration) (string, string, error) {
// 	cfg, err := createAwsClient(config)
// 	if err != nil {
// 		return "", "", err
// 	}

// 	client := ec2.NewFromConfig(cfg)

// 	// Create security group
// 	sgID, err := createSecurityGroup(ctx, client, config)
// 	if err != nil {
// 		return "", "", err
// 	}

// 	// Create IAM role
// 	roleName, err := createIAMRole(ctx, config)
// 	if err != nil {
// 		return "", "", err
// 	}

// 	// User data script
// 	userData := `#!/bin/bash
// # Update system packages
// yum update -y

// # Install Git
// yum install -y git

// # Install Node.js
// curl -sL https://rpm.nodesource.com/setup_` + config.NodeVersion + `.x | bash -
// yum install -y nodejs

// # Install development tools
// yum groupinstall -y "Development Tools"

// # Install lsof for port management
// yum install -y lsof

// # Create app directory
// mkdir -p ` + config.DeploymentPath + `
// chown ec2-user:ec2-user ` + config.DeploymentPath + `

// # Set up log directory
// mkdir -p /var/log/nodejs-app
// chown ec2-user:ec2-user /var/log/nodejs-app

// # Install useful tools
// yum install -y jq htop
// `

// 	userDataEncoded := base64.StdEncoding.EncodeToString([]byte(userData))

// 	// Launch instance
// 	runInput := &ec2.RunInstancesInput{
// 		ImageId:      aws.String(config.AmiID),
// 		InstanceType: types.InstanceType(config.InstanceType),
// 		KeyName:      aws.String(config.KeyName),
// 		MinCount:     aws.Int32(1),
// 		MaxCount:     aws.Int32(1),
// 		SecurityGroupIds: []string{
// 			sgID,
// 		},
// 		UserData: aws.String(userDataEncoded),
// 		IamInstanceProfile: &types.IamInstanceProfileSpecification{
// 			Name: aws.String(roleName),
// 		},
// 		TagSpecifications: []types.TagSpecification{
// 			{
// 				ResourceType: types.ResourceTypeInstance,
// 				Tags: []types.Tag{
// 					{
// 						Key:   aws.String("Name"),
// 						Value: aws.String(config.InstanceName),
// 					},
// 				},
// 			},
// 		},
// 	}

// 	runResult, err := client.RunInstances(ctx, runInput)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to launch EC2 instance: %v", err)
// 	}

// 	instanceID := *runResult.Instances[0].InstanceId
// 	log.Printf("Launched EC2 instance: %s", instanceID)

// 	// Wait for instance to be running
// 	log.Println("Waiting for instance to enter running state...")
// 	waiter := ec2.NewInstanceRunningWaiter(client)
// 	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{
// 		InstanceIds: []string{instanceID},
// 	}, 5*time.Minute); err != nil {
// 		return "", "", fmt.Errorf("failed while waiting for instance to run: %v", err)
// 	}

// 	// Get instance details
// 	describeResult, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
// 		InstanceIds: []string{instanceID},
// 	})

// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to describe instance: %v", err)
// 	}

// 	publicIP := *describeResult.Reservations[0].Instances[0].PublicIpAddress
// 	publicDNS := *describeResult.Reservations[0].Instances[0].PublicDnsName

// 	log.Printf("Instance is running with IP: %s, DNS: %s", publicIP, publicDNS)

// 	return publicIP, instanceID, nil
// }

// // setupGithubSecrets sets up the secrets in GitHub repository
// func setupGithubSecrets(ctx context.Context, config Configuration, publicIP, privateKeyPath string) error {
// 	// Create GitHub client
// 	ts := oauth2.StaticTokenSource(
// 		&oauth2.Token{AccessToken: config.GithubToken},
// 	)
// 	tc := oauth2.NewClient(ctx, ts)
// 	client := github.NewClient(tc)

// 	// Read SSH key file
// 	privateKey, err := ioutil.ReadFile(privateKeyPath)
// 	if err != nil {
// 		return fmt.Errorf("failed to read private key file: %v", err)
// 	}

// 	// Set required secrets
// 	secrets := map[string]string{
// 		"AWS_ACCESS_KEY_ID":     config.AwsAccessKey,
// 		"AWS_SECRET_ACCESS_KEY": config.AwsSecretKey,
// 		"AWS_REGION":            config.AwsRegion,
// 		"EC2_HOST":              publicIP,
// 		"EC2_USER":              "ec2-user",
// 		"EC2_SSH_KEY":           string(privateKey),
// 	}

// 	for name, value := range secrets {
// 		// Create or update secret
// 		_, _, err := client.Actions.CreateOrUpdateRepoSecret(
// 			ctx,
// 			config.GithubOwner,
// 			config.GithubRepo,
// 			&github.EncryptedSecret{
// 				Name:           name,
// 				EncryptedValue: value, // Note: This is simplified; GitHub API requires encryption
// 				KeyID:          "",    // Public key ID needed in real implementation
// 			},
// 		)

// 		if err != nil {
// 			return fmt.Errorf("failed to set GitHub secret %s: %v", name, err)
// 		}

// 		log.Printf("Set GitHub secret: %s", name)
// 	}

// 	return nil
// }

// func main() {
// 	log.Println("Starting EC2 setup and GitHub integration process...")

// 	// Load configuration from environment file
// 	config, err := loadEnvConfig(".env")
// 	if err != nil {
// 		log.Fatalf("Failed to load configuration: %v", err)
// 	}

// 	ctx := context.Background()

// 	// Launch EC2 instance
// 	publicIP, instanceID, err := launchEC2Instance(ctx, config)
// 	if err != nil {
// 		log.Fatalf("Failed to launch EC2 instance: %v", err)
// 	}

// 	log.Printf("EC2 instance launched successfully: %s (%s)", instanceID, publicIP)

// 	// Set up GitHub secrets (if private key path is provided)
// 	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH")
// 	if privateKeyPath != "" {
// 		if err := setupGithubSecrets(ctx, config, publicIP, privateKeyPath); err != nil {
// 			log.Fatalf("Failed to set up GitHub secrets: %v", err)
// 		}
// 		log.Println("GitHub secrets set up successfully")
// 	} else {
// 		log.Println("PRIVATE_KEY_PATH not provided, skipping GitHub secrets setup")
// 		log.Println("To complete setup manually, add these secrets to your GitHub repository:")
// 		log.Printf("- AWS_ACCESS_KEY_ID: (your access key)")
// 		log.Printf("- AWS_SECRET_ACCESS_KEY: (your secret key)")
// 		log.Printf("- AWS_REGION: %s", config.AwsRegion)
// 		log.Printf("- EC2_HOST: %s", publicIP)
// 		log.Printf("- EC2_USER: ec2-user")
// 		log.Printf("- EC2_SSH_KEY: (content of your %s.pem private key file)", config.KeyName)
// 	}

// 	log.Println("Setup complete! Your EC2 instance is ready for GitHub Actions deployment.")
// }