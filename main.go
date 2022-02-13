package main

import (
	"encoding/json"
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

const profile = "default"
const region = "eu-west-1"
const index_file = "/tmp/hosts"

// InstanceStruct represents the data structure for the instances retrieve from EC2
type InstanceStruct struct {
	InstanceID string
	Name       string
}

func connectSession() *session.Session {
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
	result, err := ec2Svc.DescribeInstances(nil)

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

func startSsm(instance string) {
	cmd := exec.Command("aws", "--profile", profile, "--region", region, "ssm", "start-session", "--target", strings.TrimSpace(instance))
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func readInput(data []InstanceStruct) string {
	fmt.Println("#########################################")
	fmt.Println("Instances")
	fmt.Println("#########################################")

	var options []string

	for _, value := range data {
		options = append(options, fmt.Sprintf("%s-%s\n", value.Name, value.InstanceID[2:]))
	}
	fmt.Print(strings.Join(options, "\n"))

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
		return fmt.Sprintf("i-%s", opt)
	} else {
		panic(err)
	}
}

func writeCache(data []InstanceStruct) {
	file, _ := json.MarshalIndent(data, "", " ")
	_ = ioutil.WriteFile(index_file, file, 0644)
}

func readCache() []InstanceStruct {
	file, err := ioutil.ReadFile(index_file)
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
	file, err := os.Stat(index_file)
	check(err)

	now := time.Now()
	instances := make([]InstanceStruct, 0)

	if file.ModTime().Unix()+3*60*60 > now.Unix() {
		instances = readCache()
	} else {
		sess := connectSession()
		instances = fetchInstances(sess)
		writeCache(instances)
	}

	instance := readInput(instances)
	startSsm(instance)
}
