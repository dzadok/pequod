package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type model struct {
	ctx   context.Context
	cli   *client.Client
	table table.Model
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.table.Focused() {
				m.table.Blur()
			} else {
				m.table.Focus()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			// TODO: figure out capturing input for env var name and value
			// then call updateEnv with m.table.SelectedRow()[0]
			return m, m.updateEnvCmd()
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return baseStyle.Render(m.table.View()) + "\n"
}

func main() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	if len(os.Args) > 1 {
		containerName := os.Args[1]
		envVar := os.Args[2]
		containers, err := cli.ContainerList(ctx,
			container.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "/"+containerName))},
		)
		if err != nil {
			panic(err)
		}
		if len(containers) == 0 {
			panic("No containers found")
		}
		var ids []string
		for _, v := range containers {
			ids = append(ids, v.ID)
		}
		err = updateEnv(ctx, cli, ids, envVar)
		if err != nil {
			panic(err)
		}
		fmt.Println("OK")

	} else {

		containers, err := cli.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			panic(err)
		}

		columns := []table.Column{
			{Title: "ID", Width: 12},
			{Title: "Name", Width: 30},
			{Title: "Command", Width: 40},
		}

		rows := []table.Row{}

		for _, v := range containers {
			name, _ := strings.CutPrefix(strings.Join(v.Names, ", "), "/")
			rows = append(
				rows,
				table.Row{
					v.ID,
					name,
					v.Command,
				},
			)
		}

		t := table.New(
			table.WithColumns(columns),
			table.WithRows(rows),
			table.WithFocused(true),
			table.WithWidth(120),
		)

		s := table.DefaultStyles()
		t.SetStyles(s)

		m := model{ctx, cli, t}
		if _, err := tea.NewProgram(m).Run(); err != nil {
			panic(err)
		}
	}
}

func (m model) updateEnvCmd() tea.Cmd {
	err := updateEnv(m.ctx, m.cli, []string{m.table.SelectedRow()[0]}, "TEST=tea")
	if err != nil {
		panic(err)
	}
	return nil
}

// TODO: Update model with new container
func updateEnv(ctx context.Context, cli *client.Client, ids []string, envVar string) error {
	varName := strings.Split(envVar, "=")[0]
	for _, v := range ids {
		oldContainer, err := cli.ContainerInspect(ctx, v)
		if err != nil {
			return err
		}
		name := oldContainer.Name
		found := false
		for i, e := range oldContainer.Config.Env {
			if strings.Split(e, "=")[0] == varName {
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
			return err
		}
		newContainer, err := cli.ContainerCreate(ctx,
			oldContainer.Config,
			oldContainer.HostConfig,
			&network.NetworkingConfig{EndpointsConfig: oldContainer.NetworkSettings.Networks},
			nil,
			"tempname")
		if err != nil {
			return err
		}
		err = cli.ContainerRemove(ctx, oldContainer.ID, container.RemoveOptions{Force: true})
		if err != nil {
			return err
		}
		err = cli.ContainerRename(ctx, newContainer.ID, name)
		if err != nil {
			return err
		}
		err = cli.ContainerStart(ctx, newContainer.ID, container.StartOptions{})
		if err != nil {
			return err
		}
		println(fmt.Sprintf("Restarted %s", name))
	}
	return nil
}
