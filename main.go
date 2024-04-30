package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

func main() {
	containerName := os.Args[1]
	envVar := os.Args[2]
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()
	containers, err := cli.ContainerList(ctx,
		container.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "/"+containerName))},
	)
	if err != nil {
		panic(err)
	}
	if len(containers) == 0 {
		panic("No containers found")
	}
	updateEnv(ctx, cli, containers, envVar)
	fmt.Println("OK")
}

func updateEnv(ctx context.Context, cli *client.Client, containers []types.Container, envVar string) {
	varName := strings.Split(envVar, "=")[0]
	for _, v := range containers {
		id := v.ID
		oldContainer, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			panic(err)
		}
		name := oldContainer.Name
		found := false
		for i, v := range oldContainer.Config.Env {
			if strings.Split(v, "=")[0] == varName {
				oldContainer.Config.Env[i] = envVar
				found = true
				break
			}
		}
		if !found {
			oldContainer.Config.Env = append(oldContainer.Config.Env, envVar)
		}
		err = cli.ContainerStop(ctx, oldContainer.ID, container.StopOptions{})
		if err != nil {
			panic(err)
		}
		newContainer, err := cli.ContainerCreate(ctx,
			oldContainer.Config,
			oldContainer.HostConfig,
			&network.NetworkingConfig{EndpointsConfig: v.NetworkSettings.Networks},
			nil,
			"tempname")
		if err != nil {
			panic(err)
		}
		err = cli.ContainerRemove(ctx, oldContainer.ID, container.RemoveOptions{Force: true})
		if err != nil {
			panic(err)
		}
		err = cli.ContainerRename(ctx, newContainer.ID, name)
		if err != nil {
			panic(err)
		}
		err = cli.ContainerStart(ctx, newContainer.ID, container.StartOptions{})
		if err != nil {
			panic(err)
		}
		println(fmt.Sprintf("Restarted %s", name))
	}
}
