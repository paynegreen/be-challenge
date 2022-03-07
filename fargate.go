package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/peterh/liner"
)

// FargateStruct represents the data structure for the instances retrieve from ECS Fargate
type FargateStruct struct {
	Name string
}

func fetchClusters(client *session.Session) []FargateStruct {
	ecsSvc := ecs.New(client)
	params := &ecs.ListClustersInput{}
	result, err := ecsSvc.ListClusters(params)

	if err != nil {
		check(err)
	}

	var clusters []FargateStruct
	for _, c := range result.ClusterArns {
		r, _ := regexp.Compile("cluster/")
		split := r.Split(*c, -1)
		cluster := split[len(split)-1]
		clusters = append(clusters, FargateStruct{cluster})
	}

	return clusters
}

func fetchServices(client *session.Session, cluster string) []FargateStruct {
	ecsSvc := ecs.New(client)
	params := &ecs.ListServicesInput{
		Cluster: aws.String(cluster),
	}
	result, err := ecsSvc.ListServices(params)

	if err != nil {
		check(err)
	}

	var services []FargateStruct
	for _, s := range result.ServiceArns {
		r, _ := regexp.Compile(fmt.Sprintf("service/%s/", cluster))
		split := r.Split(*s, -1)
		service := split[len(split)-1]
		services = append(services, FargateStruct{service})
	}

	return services
}

func fetchTasks(client *session.Session, cluster string, service string) []FargateStruct {

	ecsSvc := ecs.New(client)
	params := &ecs.ListTasksInput{
		Cluster:     aws.String(cluster),
		ServiceName: aws.String(service),
	}
	result, err := ecsSvc.ListTasks(params)

	if err != nil {
		check(err)
	}

	var tasks []FargateStruct
	for _, s := range result.TaskArns {
		r, _ := regexp.Compile(fmt.Sprintf("task/%s/", cluster))
		split := r.Split(*s, -1)
		service := split[len(split)-1]
		tasks = append(tasks, FargateStruct{service})
	}

	return tasks
}

func describeTasks(client *session.Session, cluster string, task string) []FargateStruct {
	ecsSvc := ecs.New(client)
	params := &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []*string{aws.String(task)},
	}
	result, err := ecsSvc.DescribeTasks(params)

	if err != nil {
		check(err)
	}

	var tasks []FargateStruct
	for _, t := range result.Tasks[0].Containers {
		tasks = append(tasks, FargateStruct{*t.Name})
	}

	return tasks
}

func startFargateSsm(cluster string, task string, container string, profile string, region string) {
	fmt.Println("#########################################")
	fmt.Println("Exec into", cluster, task, container)
	fmt.Println("#########################################")

	command := "/bin/bash"
	cmd := exec.Command("aws", "--profile", profile, "--region", region, "ecs", "execute-command", "--cluster", strings.TrimSpace(cluster), "--task", strings.TrimSpace(task), "--container", strings.TrimSpace(container), "--command", command, "--interactive")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func readFargateInput(data []FargateStruct) (string, error) {
	fmt.Println("#########################################")
	fmt.Println("Fargate")
	fmt.Println("#########################################")

	var options []string

	for _, value := range data {
		options = append(options, fmt.Sprintf("%s", value.Name))
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
		r, _ := regexp.Compile("cluster-")
		inputs := r.Split(opt, -1)
		opt = strings.TrimSpace(inputs[len(inputs)-1])
		if opt == "exit" {
			os.Exit(3)
		}
		return opt, nil
	}

	return "error", errors.New("error parsing input")
}
