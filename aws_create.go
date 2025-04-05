package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/spf13/viper"
)

func awsCreateMain() {
	// Initialize Viper for configuration
	setupViper()

	// Load AWS credentials from environment variables set by Viper
	awsAccessKey := viper.GetString("AWS_ACCESS_KEY_ID")
	awsSecretKey := viper.GetString("AWS_SECRET_ACCESS_KEY")
	awsRegion := viper.GetString("AWS_REGION")

	if awsAccessKey == "" || awsSecretKey == "" || awsRegion == "" {
		log.Fatal("AWS credentials or region not found in environment file")
	}

	// Create AWS session with credentials
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(awsRegion),
		Credentials: credentials.NewStaticCredentials(awsAccessKey, awsSecretKey, ""),
	})

	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}

	// Create EC2 service client
	ec2Svc := ec2.New(sess)

	// Define EC2 instance parameters
	instanceType := viper.GetString("EC2_INSTANCE_TYPE")
	if instanceType == "" {
		instanceType = "t2.micro" // Default instance type
	}

	// Get AMI ID - first try from env file, then try to fetch latest Amazon Linux
	amiID := viper.GetString("EC2_AMI_ID")
	if amiID == "" {
		// Try to get the latest Amazon Linux AMI from SSM parameter store
		ssmSvc := ssm.New(sess)
		amiID, err = getLatestAmazonLinuxAMI(ssmSvc)
		if err != nil {
			// Fallback to finding the latest Amazon Linux AMI via EC2 API
			amiID, err = findLatestAmazonLinuxAMI(ec2Svc)
			if err != nil {
				log.Fatalf("Failed to get AMI ID: %v", err)
			}
		}
	}

	fmt.Printf("Using AMI: %s\n", amiID)
	fmt.Printf("Instance Type: %s\n", instanceType)

	// Create the EC2 instance
	runResult, err := ec2Svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: aws.String(instanceType),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
	})

	if err != nil {
		log.Fatalf("Failed to create EC2 instance: %v", err)
	}

	// Extract instance ID
	instanceID := *runResult.Instances[0].InstanceId

	// Add a name tag to the instance
	instanceName := viper.GetString("EC2_INSTANCE_NAME")
	if instanceName == "" {
		instanceName = "GoCreatedInstance"
	}

	_, err = ec2Svc.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{aws.String(instanceID)},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(instanceName),
			},
		},
	})

	if err != nil {
		log.Printf("Failed to tag EC2 instance: %v", err)
	}

	fmt.Printf("Successfully created EC2 instance with ID: %s and Name: %s\n", instanceID, instanceName)
}

// Get the latest Amazon Linux AMI using the AWS Systems Manager Parameter Store
func getLatestAmazonLinuxAMI(ssmSvc *ssm.SSM) (string, error) {
	// This SSM parameter always points to the latest Amazon Linux 2 AMI
	paramName := "/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2"

	result, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name: aws.String(paramName),
	})

	if err != nil {
		return "", fmt.Errorf("failed to get AMI from SSM: %v", err)
	}

	return *result.Parameter.Value, nil
}

// Find the latest Amazon Linux AMI using the EC2 API (fallback method)
func findLatestAmazonLinuxAMI(ec2Svc *ec2.EC2) (string, error) {
	input := &ec2.DescribeImagesInput{
		Owners: []*string{aws.String("amazon")},
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("name"),
				Values: []*string{aws.String("amzn2-ami-hvm-*-x86_64-gp2")},
			},
			{
				Name:   aws.String("state"),
				Values: []*string{aws.String("available")},
			},
		},
	}

	result, err := ec2Svc.DescribeImages(input)
	if err != nil {
		return "", fmt.Errorf("failed to describe images: %v", err)
	}

	if len(result.Images) == 0 {
		return "", fmt.Errorf("no Amazon Linux AMIs found")
	}

	// Find the most recent AMI
	var latestImage *ec2.Image
	var latestTime time.Time

	for _, image := range result.Images {
		creationDate, err := time.Parse(time.RFC3339, *image.CreationDate)
		if err != nil {
			continue
		}

		if latestImage == nil || creationDate.After(latestTime) {
			latestImage = image
			latestTime = creationDate
		}
	}

	if latestImage == nil {
		return "", fmt.Errorf("failed to find latest AMI")
	}

	return *latestImage.ImageId, nil
}

func setupViper() {
	// Set the file name of the configuration file
	viper.SetConfigName(".env") // name of config file (without extension)
	viper.SetConfigType("env")  // REQUIRED if the config file does not have the extension in the name

	// Add search paths
	viper.AddConfigPath(".")

	// Read environment file
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Set Viper to also read from OS environment variables
	viper.AutomaticEnv()

	// Optional: Print loaded config for debugging
	fmt.Println("Configuration loaded successfully")
}
