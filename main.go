package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func main() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		panic(err)
	}
	id := containers[2].ID
	oldContainer, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		panic(err)
	}
	name := oldContainer.Name
	oldContainer.Config.Env = append(oldContainer.Config.Env, "TEST=hello")
	err = cli.ContainerStop(ctx, oldContainer.ID, container.StopOptions{})
	if err != nil {
		panic(err)
	}
	newContainer, err := cli.ContainerCreate(ctx,
		oldContainer.Config,
		oldContainer.HostConfig,
		&network.NetworkingConfig{EndpointsConfig: containers[2].NetworkSettings.Networks},
		&v1.Platform{},
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
	fmt.Println("OK")
}
