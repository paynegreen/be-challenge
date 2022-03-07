package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/peterh/liner"
)

const indexFile = "/tmp/hosts"

// InstanceStruct represents the data structure for the instances retrieve from EC2
type InstanceStruct struct {
	InstanceID string
	Name       string
}

func connectSession(profile string, region string) *session.Session {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", profile),
	})

	if err != nil {
		check(err)
	}

	return sess
}

func fetchInstances(client *session.Session) []InstanceStruct {
	ec2Svc := ec2.New(client)
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			},
		},
	}
	result, err := ec2Svc.DescribeInstances(params)

	if err != nil {
		check(err)
	}

	var instances []InstanceStruct
	for _, r := range result.Reservations {
		for _, instance := range r.Instances {
			var name string
			for _, tag := range instance.Tags {
				if *tag.Key == "Name" {
					name = *tag.Value
					break
				}
			}
			instances = append(instances, InstanceStruct{*instance.InstanceId, name})
		}
	}

	return instances
}

func startSsm(instance string, profile string, region string) {
	cmd := exec.Command("aws", "--profile", profile, "--region", region, "ssm", "start-session", "--target", strings.TrimSpace(instance))
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func readInput(data []InstanceStruct) (string, error) {
	fmt.Println("#########################################")
	fmt.Println("Instances")
	fmt.Println("#########################################")

	var options []string

	for _, value := range data {
		options = append(options, fmt.Sprintf("%s-%s", value.Name, value.InstanceID[2:]))
	}
	fmt.Println(strings.Join(options, "\n"))

	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)

	line.SetCompleter(func(line string) (c []string) {
		for _, opt := range options {
			if strings.HasPrefix(opt, strings.ToLower(line)) {
				c = append(c, opt)
			}
		}
		return
	})

	if opt, err := line.Prompt("> "); err == nil {
		inputs := strings.Split(opt, "-")
		opt = strings.TrimSpace(inputs[len(inputs)-1])
		if opt == "exit" {
			os.Exit(3)
		}
		return fmt.Sprintf("i-%s", opt), nil
	}

	return "error", errors.New("error parsing input")
}

func writeCache(data []InstanceStruct) {
	file, _ := json.MarshalIndent(data, "", " ")
	_ = ioutil.WriteFile(indexFile, file, 0644)
}

func readCache() []InstanceStruct {
	file, err := ioutil.ReadFile(indexFile)
	check(err)
	instances := make([]InstanceStruct, 0)
	json.Unmarshal(file, &instances)

	return instances
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	profile := flag.String("profile", "default", "Change default AWS_PROFILE")
	region := flag.String("region", "eu-west-1", "Change default AWS_REGION")
	rebuild := flag.String("rebuild", "false", "Set rebuild to clear cached resources")
	resource := flag.String("resource", "ec2", "Retrieve EC2 or ECS Fargate resources")
	flag.Parse()

	sess := connectSession(*profile, *region)

	if *resource == "ec2" {
		file, _ := os.Stat(indexFile)

		now := time.Now()
		instances := make([]InstanceStruct, 0)

		if *rebuild == "false" && file != nil && file.ModTime().Unix()+3*60*60 > now.Unix() {
			instances = readCache()
		} else {
			instances = fetchInstances(sess)
			writeCache(instances)
		}
		instance, err := readInput(instances)
		check(err)
		startSsm(instance, *profile, *region)
	} else {
		clusters := make([]FargateStruct, 0)
		clusters = fetchClusters(sess)
		cluster, err := readFargateInput(clusters)

		check(err)
		services := fetchServices(sess, cluster)
		service, err := readFargateInput(services)
		check(err)
		tasks := fetchTasks(sess, cluster, service)
		task, err := readFargateInput(tasks)
		check(err)
		containers := describeTasks(sess, cluster, task)
		container, err := readFargateInput(containers)
		check(err)
		startFargateSsm(cluster, task, container, *profile, *region)
	}

}
