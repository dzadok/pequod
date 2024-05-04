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
	ctx        context.Context
	cli        *client.Client
	containers table.Model
	envs       table.Model
	showEnvs   bool
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.containers.Focused() {
				m.containers.Blur()
			} else {
				m.containers.Focus()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			return m, m.displayEnv()
		case "enter":
			// TODO: figure out capturing input for env var name and value
			// then call updateEnv with m.table.SelectedRow()[0]
			return m, m.updateEnvCmd()
		}
	}
	if m.showEnvs == true {
		m.envs, cmd = m.envs.Update(msg)
	} else {
		m.containers, cmd = m.containers.Update(msg)
	}
	return m, cmd
}

func (m *model) View() string {
	if m.showEnvs == true {
		return baseStyle.Render(m.envs.View()) + "\n"
	}
	return baseStyle.Render(m.containers.View()) + "\n"
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
		_, err = updateEnv(ctx, cli, ids, envVar)
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

		m := new(model)
		m.cli = cli
		m.ctx = ctx
		m.containers = t
		m.envs = table.New(
			table.WithColumns([]table.Column{
				{Title: "Name", Width: 33},
				{Title: "Value", Width: 33},
			}),
			table.WithRows([]table.Row{}),
			table.WithWidth(80),
		)
		m.showEnvs = false
		if _, err := tea.NewProgram(m).Run(); err != nil {
			panic(err)
		}
	}
}

func (m *model) displayEnv() tea.Cmd {
	m.showEnvs = true
	c := m.containers.SelectedRow()[0]
	j, err := m.cli.ContainerInspect(m.ctx, c)
	if err != nil {
		panic(err)
	}
	rows := []table.Row{}
	for _, e := range j.Config.Env {
		estring := strings.Split(e, "=")
		rows = append(rows, table.Row{estring[0], estring[1]})

	}
	m.envs.SetRows(rows)
	m.envs.Update(nil)

	return nil
}

func (m *model) updateEnvCmd() tea.Cmd {
	m.showEnvs = false
	o := m.containers.SelectedRow()[0]
	ids, err := updateEnv(m.ctx, m.cli, []string{m.containers.SelectedRow()[0]}, "TEST=tea")
	if err != nil {
		panic(err)
	}
	newRows := []table.Row{}
	for _, v := range m.containers.Rows() {
		if v[0] == o {
			v[0] = ids[0]
		}
		newRows = append(newRows, v)
	}
	m.containers.SetRows(newRows)
	return nil
}

func updateEnv(ctx context.Context, cli *client.Client, ids []string, envVar string) ([]string, error) {
	varName := strings.Split(envVar, "=")[0]
	i := []string{}
	for _, v := range ids {
		oldContainer, err := cli.ContainerInspect(ctx, v)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		newContainer, err := cli.ContainerCreate(ctx,
			oldContainer.Config,
			oldContainer.HostConfig,
			&network.NetworkingConfig{EndpointsConfig: oldContainer.NetworkSettings.Networks},
			nil,
			"tempname")
		if err != nil {
			return nil, err
		}
		err = cli.ContainerRemove(ctx, oldContainer.ID, container.RemoveOptions{Force: true})
		if err != nil {
			return nil, err
		}
		err = cli.ContainerRename(ctx, newContainer.ID, name)
		if err != nil {
			return nil, err
		}
		err = cli.ContainerStart(ctx, newContainer.ID, container.StartOptions{})
		if err != nil {
			return nil, err
		}
		i = append(i, newContainer.ID)
		// println(fmt.Sprintf("Restarted %s", name))
	}
	return i, nil
}
