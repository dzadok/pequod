package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type mainModel struct {
	cli        *client.Client
	containers table.Model
	envs       envModel
	showEnvs   bool
	error      error
	spinner    *spinner.Model
}

type envModel struct {
	cli       *client.Client
	container string
	envs      table.Model
	n         string
	v         string
	spinner   *spinner.Model
}

type showEnv struct{}

type showContainers struct{}

type updateContainers struct {
	containers table.Model
}

type display struct {
	e envModel
}

func (m mainModel) Init() tea.Cmd { return nil }

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showEnvs == true {
			m.envs, cmd = m.envs.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "esc":
			if m.showEnvs == true {
				// 	m.showEnvs = false
				// 	m.spinner = nil
				// 	return m, nil
				m.showEnvs = false
				return m, nil
			}
			return m, tea.Quit
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "enter":
			if m.showEnvs == true {
				// 	m.showEnvs = false
				// 	m.envs, cmd = m.envs.Update(msg)
				// 	return m, cmd
			}
			m.showEnvs = true
			s := spinner.New()
			m.spinner = &s
			return m, tea.Batch(m.spinner.Tick, m.newEnvModel)
		}
	case showEnv:
		m.spinner = nil
		m.showEnvs = true
		// m.envs, cmd = m.envs.Update(msg)
		return m, cmd
	case showContainers:
		m.spinner = nil
		m.showEnvs = false
		return m, m.updateContainers
	case updateContainers:
		m.containers = msg.containers
		return m, nil
	}
	if m.showEnvs == true {
		e, cmd := m.envs.Update(msg)
		m.envs = e
		return m, cmd
	}
	if m.spinner != nil {
		s, cmd := m.spinner.Update(m.spinner.Tick())
		m.spinner = &s
		return m, tea.Batch(cmd, m.updateContainers)
	}
	m.containers, cmd = m.containers.Update(msg)
	return m, cmd
}

func (m envModel) Update(msg tea.Msg) (envModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, m.exit
		case "tab", "enter":
			m.n = m.envs.SelectedRow()[0]
			m.v = m.envs.SelectedRow()[1]
			huh.NewInput().
				Title(m.n).
				Value(&m.v).
				Run()
			s := spinner.New()
			m.spinner = &s
			return m, tea.Batch(m.spinner.Tick, m.updateEnvCmd)

		// o => insert new line in vim, "a"dd, "n"ew
		case "o", "a", "n":
			var varName string
			var varValue string
			huh.NewInput().
				Title("New Variable Name").
				Value(&varName).
				Run()
			huh.NewInput().
				Title(varName).
				Value(&varValue).
				Run()
			m.n = varName
			m.v = varValue
			return m, m.updateEnvCmd
		}
	case display:
		msg.e.envs, cmd = msg.e.envs.Update(msg)
		msg.e.spinner = nil
		return msg.e, cmd
	}
	m.envs, cmd = m.envs.Update(msg)
	return m, cmd
}

func (m mainModel) View() string {
	if m.showEnvs == true {
		return m.envs.View()
	}
	if m.spinner != nil {
		return m.spinner.View()
	}
	return baseStyle.Render(m.containers.View()) + "\n"
}

func (m envModel) View() string {
	if m.spinner != nil {
		return m.spinner.View()
	}
	return baseStyle.Render(m.envs.View()) + "\n"
}

func main() {
	f, err := os.OpenFile("./log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		println("Error opening log file, will continue")
	}
	defer f.Close()
	log.SetOutput(f)
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Panicln(err)
	}
	defer cli.Close()

	if len(os.Args) > 1 {
		containerName := os.Args[1]
		envVar := os.Args[2]
		containers, err := cli.ContainerList(ctx,
			container.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "/"+containerName))},
		)
		if err != nil {
			log.Panicln(err)
		}
		if len(containers) == 0 {
			log.Println("No containers found")
			println("No containers found")
			os.Exit(1)
		}
		var ids []string
		for _, v := range containers {
			ids = append(ids, v.ID)
		}
		_, err = updateEnv(ctx, cli, ids, envVar)
		if err != nil {
			log.Println(err)
			fmt.Println("An error occurred")
			os.Exit(1)
		}
		fmt.Println("OK")

	} else {
		m := new(mainModel)
		m.cli = cli
		t := m.getContainers()
		m.containers = t
		m.showEnvs = false
		if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
			log.Panicln(err)
		}
	}
}

func (m mainModel) getContainers() table.Model {
	containers, err := m.cli.ContainerList(context.Background(), container.ListOptions{})
	if err != nil {
		log.Panicln(err)
	}

	columns := []table.Column{
		{Title: "ID", Width: 12},
		{Title: "Name", Width: 30},
		{Title: "Command", Width: 70},
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
	return t
}

func (m mainModel) updateContainers() tea.Msg {
	return updateContainers{m.getContainers()}
}

func (m mainModel) newEnvModel() tea.Msg {
	e := new(envModel)
	e.container = m.containers.SelectedRow()[0]
	e.cli = m.cli
	j, err := m.cli.ContainerInspect(context.Background(), e.container)
	if err != nil {
		log.Panicln(err)
	}
	rows := []table.Row{}
	for _, v := range j.Config.Env {
		estring := strings.Split(v, "=")
		if len(estring) == 1 {
			rows = append(rows, table.Row{estring[0], ""})
		} else {
			rows = append(rows, table.Row{estring[0], estring[1]})
		}

	}
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 30},
			{Title: "Value", Width: 85},
		}),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithWidth(120),
	)
	t.GotoTop()
	s := table.DefaultStyles()
	t.SetStyles(s)

	e.envs = t
	return display{*e}
}

func (m envModel) Init() tea.Msg {
	return showEnv{}
}

func (m envModel) exit() tea.Msg {
	return showContainers{}
}

// TODO: Only call updateEnv if something changed
func (m envModel) updateEnvCmd() tea.Msg {
	e := []string{}
	e = append(e, m.n)
	e = append(e, m.v)
	u := strings.Join(e, "=")
	_, err := updateEnv(context.Background(), m.cli, []string{m.container}, u)
	if err != nil {
		log.Panicln(err)
	}
	return showContainers{}
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
	}
	return i, nil
}
