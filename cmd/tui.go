package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"codeberg.org/2ug/stormdrain/internal"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

/* Кто-то ещё в саду. */

var (
	gs            *internal.GeneralStats
	version       string
)

const (
	selectionColor = tcell.Color111
	toolNameColor  = tcell.ColorYellowGreen
	titleColor     = tcell.ColorBlanchedAlmond
	headingColor   = tcell.ColorBeige
	errorColor     = tcell.ColorFireBrick
	inactiveColor  = tcell.ColorGray
)

func RunTUI(v string) {
	version = v
	app := tview.NewApplication()

	headerView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })
	headerView.SetBorder(true).
		SetTitle(" Stormdrain ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(toolNameColor)

	errorView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })

	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)
	table.SetBorder(true).
		SetTitle(" Containers ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)
	table.SetSelectedStyle(tcell.Style{}.Foreground(selectionColor))

	detailView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })
	detailView.SetBorder(true).
		SetTitle(" Detail ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(headerView, 3, 0, false).
		AddItem(errorView, 1, 0, false).
		AddItem(table, 0, 1, true).
		AddItem(detailView, 8, 0, false)

	var err error
	gs, err = internal.NewGeneralStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	updateHeader(headerView)
	populateTable(table)

	if len(gs.Containers) > 0 {
		table.Select(1, 0)
	}
	updateDetailView(detailView, table)

	// query data from podman every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := gs.Update(); err != nil {
				app.QueueUpdateDraw(func() {
					errorView.SetText(fmt.Sprintf("[red]%s[-]", err.Error()))
				})
				continue
			}
			app.QueueUpdateDraw(func() {
				errorView.SetText("")
				rebuildTable(table)
				updateDetailView(detailView, table)
			})
		}
	}()

	// redraw header section every second to update datetime
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			app.QueueUpdateDraw(func() {
				updateHeader(headerView)
			})
		}
	}()

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, _ := table.GetSelection()

		switch event.Rune() {
		case 'j':
			nextRow := row + 1
			for nextRow < table.GetRowCount() {
				if table.GetCell(nextRow, 0).GetReference() != nil {
					table.Select(nextRow, 0)
					break
				}
				nextRow++
			}
			updateDetailView(detailView, table)
			return nil
		case 'k':
			prevRow := row - 1
			for prevRow >= 1 {
				if table.GetCell(prevRow, 0).GetReference() != nil {
					table.Select(prevRow, 0)
					break
				}
				prevRow--
			}
			updateDetailView(detailView, table)
			return nil
		case 'q':
			app.Stop()
			return nil
		case 'N':
			// TODO: spawn modal for new container
			return nil
		}

		return event
	})

	if err := app.SetRoot(flex, true).EnableMouse(false).Run(); err != nil {
		panic(err)
	}
}

func updateHeader(headerView *tview.TextView) {
	if gs == nil {
		headerView.SetText(fmt.Sprintf("%s [gray]/[end] %s", time.Now().Format("2006-01-02 15:04:05"), version))
		return
	}
	headerView.SetText(fmt.Sprintf(
		"%s [gray]/[end] %s [gray]/[end] [green]%s[end] [gray]/[end] %d CPUs [gray]/[end] %d GB [gray]/[end] [yellow]%d/%d running[end]",
		time.Now().Format("2006-01-02 15:04:05"),
		version,
		gs.MachineName,
		gs.AvailableTotalCPUs,
		gs.AvailableTotalMemoryGB,
		gs.TotalRunning,
		gs.TotalContainers,
	))
}

func updateDetailView(detailView *tview.TextView, table *tview.Table) {
	row, _ := table.GetSelection()
	if row < 1 || row >= table.GetRowCount() {
		detailView.SetText("[gray]No container selected[-]")
		return
	}
	ref := table.GetCell(row, 0).GetReference()
	if ref == nil {
		detailView.SetText("[gray]No container selected[-]")
		return
	}
	idx, ok := ref.(int)
	if !ok || idx < 0 || idx >= len(gs.Containers) {
		detailView.SetText("[gray]No container selected[-]")
		return
	}
	detailView.SetText(strings.Join(formatDetail(&gs.Containers[idx]), "\n"))
}

func rebuildTable(table *tview.Table) {
	row, _ := table.GetSelection()

	var savedContainerIdx int
	if row >= 0 && row < table.GetRowCount() {
		ref := table.GetCell(row, 0).GetReference()
		if ref != nil {
			if idx, ok := ref.(int); ok {
				savedContainerIdx = idx
			}
		}
	}

	populateTable(table)

	if savedContainerIdx >= 0 && savedContainerIdx < len(gs.Containers) {
		targetRow := findRowForContainer(table, savedContainerIdx)
		if targetRow > 0 {
			table.Select(targetRow, 0)
			return
		}
	}

	if table.GetRowCount() > 1 {
		table.Select(1, 0)
	}
}

func populateTable(table *tview.Table) {
	table.Clear()

	headers := []string{"Name", "Status", "CPU (dir/avg)", "Memory"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetSelectable(false).
			SetTextColor(headingColor).
			SetExpansion(1)
		table.SetCell(0, i, cell)
	}

	rowIdx := 1
	for i, c := range gs.Containers {
		status := statusString(c.Uptime)
		data := []string{c.Name, status, c.CPU, c.Memory}
		for col, text := range data {
			color := tcell.ColorWhite
			if c.Uptime == -1 {
				color = inactiveColor
			}
			cell := tview.NewTableCell(text).SetReference(i).SetTextColor(color).SetExpansion(1)
			table.SetCell(rowIdx, col, cell)
		}
		rowIdx++
	}
}

func findRowForContainer(table *tview.Table, containerIdx int) int {
	for r := 1; r < table.GetRowCount(); r++ {
		ref := table.GetCell(r, 0).GetReference()
		if ref != nil {
			if idx, ok := ref.(int); ok && idx == containerIdx {
				return r
			}
		}
	}
	return -1
}

func statusString(uptime int) string {
	if uptime < 0 {
		return "down"
	}
	d := time.Duration(uptime) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds up", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm up", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh up", int(d.Hours()))
	}
	return fmt.Sprintf("%dd up", int(d.Hours()/24))
}

func formatDetail(c *internal.ContainerStats) []string {
	var lines []string

	lines = append(lines, fmt.Sprintf("Image: %s", c.ImageTag))
	lines = append(lines, fmt.Sprintf("Project: %s", c.ProjectPath))

	if len(c.Ports) > 0 {
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, p.Protocol))
		}
		lines = append(lines, fmt.Sprintf("Ports: %s", strings.Join(ports, ", ")))
	} else {
		lines = append(lines, "Ports: (none)")
	}

	if len(c.Mounts) > 0 {
		lines = append(lines, fmt.Sprintf("Mounts: %s", strings.Join(c.Mounts, ", ")))
	} else {
		lines = append(lines, "Mounts: (none)")
	}

	if c.NetIO != "" {
		lines = append(lines, fmt.Sprintf("Net I/O: %s", c.NetIO))
	}

	return lines
}
