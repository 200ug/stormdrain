package tui

import (
	"fmt"
	"sort"
	"time"

	"codeberg.org/2ug/stormdrain/internal/manager"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

/* Кто-то ещё в саду. */

type NaviDirection int

const (
	NaviUp NaviDirection = iota
	NaviDown
)

const (
	// TODO: adjust these to make sure they all fit together nicely
	defaultTextColor = tcell.ColorWhite
	inactiveColor    = tcell.ColorGray
	headingColor     = tcell.ColorBeige
	titleColor       = tcell.ColorBlanchedAlmond
	toolNameColor    = tcell.ColorYellowGreen
	selectionColor   = tcell.Color111
)

type TUI struct {
	VersionCode      string
	ActiveRow        int
	DataManager      *manager.Manager
	App              *tview.Application
	HeaderView       *tview.TextView
	NotificationView *tview.TextView
	ContainerTable   *tview.Table
	DetailView       *tview.TextView
}

func NewTUI(manager *manager.Manager, versionCode string) *TUI {
	var tui *TUI
	app := tview.NewApplication()

	headerView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })
	headerView.SetBorder(true).
		SetTitle(" Stormdrain ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(toolNameColor)

	notificationView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })

	containerTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)
	containerTable.SetBorder(true).
		SetTitle(" Containers ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)
	containerTable.SetSelectedStyle(tcell.Style{}.Foreground(selectionColor))

	detailView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { app.Draw() })
	detailView.SetBorder(true).
		SetTitle(" Detail ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(headerView, 3, 0, false).
		AddItem(notificationView, 1, 0, false).
		AddItem(containerTable, 0, 1, true).
		AddItem(detailView, 8, 0, false)
	app.SetRoot(flex, true).EnableMouse(false)

	containerTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'j':
			tui.navigateTable(NaviDown)
			tui.updateDetails()
			return nil
		case 'k':
			tui.navigateTable(NaviUp)
			tui.updateDetails()
			return nil
		case 'q':
			app.Stop()
			return nil
		case 'N':
			// TODO: modal (or similar) for new container
			//		 the command needs to be sent to m.CmdChan
			return nil
		default:
			return event
		}
	})

	tui = &TUI{
		VersionCode:      versionCode,
		ActiveRow:        0, // header row, i.e. no selection
		DataManager:      manager,
		App:              app,
		HeaderView:       headerView,
		NotificationView: notificationView,
		ContainerTable:   containerTable,
		DetailView:       detailView,
	}

	return tui
}

func (t *TUI) Run() error {
	// render initial data before entering ticker loop to prevent stale ui
	t.updateHeader()
	t.updateContainerTable()
	t.updateDetails()

	stopChan := make(chan any) // don't buffer this one
	defer close(stopChan)
	go t.DataManager.Run(stopChan) // polls new data every 5s
	go t.Update(stopChan)
	return t.App.Run()
}

func (t *TUI) Update(stopChan chan any) {
	datetimeTicker := time.NewTicker(1 * time.Second)
	dataTicker := time.NewTicker(5 * time.Second)
	defer datetimeTicker.Stop()
	defer dataTicker.Stop()
	for {
		select {
		case <-stopChan:
			return
		case <-datetimeTicker.C:
			t.App.QueueUpdateDraw(func() {
				t.updateHeader()
			})
		case <-dataTicker.C:
			t.App.QueueUpdateDraw(func() {
				t.updateContainerTable()
				t.updateDetails()
			})
		}
	}
}

func (t *TUI) updateHeader() {
	t.DataManager.Mu.RLock()
	t.HeaderView.SetText(fmt.Sprintf(
		"%s [gray]/[-] %s [gray]/[-] [green]%s[-] [gray]/[-] %d CPUs [gray]/[-] %d GB [gray]/[-] [yellow]%d container(s)[-]",
		time.Now().Format("2006-01-02 15:04:05"), // dynamic
		t.VersionCode,
		t.DataManager.PodmanStats.MachineName,
		t.DataManager.PodmanStats.AvailableTotalCPUs,
		t.DataManager.PodmanStats.AvailableTotalMemoryGB,
		len(t.DataManager.Containers), // dynamic
	))
	t.DataManager.Mu.RUnlock()
}

func (t *TUI) updateDetails() {
	if t.ActiveRow < 1 || t.ActiveRow >= t.ContainerTable.GetRowCount() {
		t.DetailView.SetText("No container selected").SetTextColor(inactiveColor)
		return
	}
	ref := t.ContainerTable.GetCell(t.ActiveRow, 0).GetReference()
	if ref == nil {
		t.DetailView.SetText("No container selected").SetTextColor(inactiveColor)
		return
	}
	name, ok := ref.(string)
	if !ok {
		t.DetailView.SetText("No container selected").SetTextColor(inactiveColor)
		return
	}
	t.DataManager.Mu.RLock()
	var container manager.Container
	found := false
	for _, c := range t.DataManager.Containers {
		if c.Name == name {
			container = c
			found = true
			break
		}
	}
	t.DataManager.Mu.RUnlock()
	if !found {
		t.DetailView.SetText("No container selected").SetTextColor(inactiveColor)
		return
	}
	t.DetailView.SetText(container.FormatDetails()).SetTextColor(defaultTextColor)
}

func (t *TUI) updateContainerTable() {
	selectedName := ""
	if t.ActiveRow >= 1 && t.ActiveRow < t.ContainerTable.GetRowCount() {
		if ref := t.ContainerTable.GetCell(t.ActiveRow, 0).GetReference(); ref != nil {
			if name, ok := ref.(string); ok {
				selectedName = name
			}
		}
	}

	// TODO: only re-draw the table *if* there's running containers (i.e.
	//		 we still need to query data, but not necessarily clear and draw)

	t.ContainerTable.Clear()

	t.DataManager.Mu.RLock()
	containers := make([]manager.Container, len(t.DataManager.Containers))
	copy(containers, t.DataManager.Containers)
	t.DataManager.Mu.RUnlock()
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	headers := []string{"Name", "Status", "CPU (dir/avg)", "Memory"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetSelectable(false).
			SetTextColor(headingColor).
			SetExpansion(1)
		t.ContainerTable.SetCell(0, i, cell)
	}

	for i, c := range containers {
		data := []string{c.Name, c.StatusString(), c.CPU, c.Memory}
		for col, text := range data {
			color := defaultTextColor
			if c.UptimeSec == -1 {
				color = inactiveColor
			}
			// expand all cells to spread the table contents evenly across screen's width
			cell := tview.NewTableCell(text).SetReference(c.Name).SetTextColor(color).SetExpansion(1)
			t.ContainerTable.SetCell(i+1, col, cell) // i+1 due to row 0 being reserved for headers
		}
	}

	t.restoreActiveSelection(selectedName)
}

func (t *TUI) restoreActiveSelection(selectedName string) {
	if t.ContainerTable.GetRowCount() < 2 {
		t.ActiveRow = 0
		return
	}
	for row := 1; row < t.ContainerTable.GetRowCount(); row++ {
		if ref := t.ContainerTable.GetCell(row, 0).GetReference(); ref != nil {
			if name, ok := ref.(string); ok && name == selectedName {
				t.ActiveRow = row
				t.ContainerTable.Select(row, 0)
				return
			}
		}
	}
	t.ActiveRow = 1
	t.ContainerTable.Select(1, 0)
}

func (t *TUI) navigateTable(direction NaviDirection) {
	if direction == NaviUp {
		prevRow := t.ActiveRow - 1
		for prevRow >= 1 {
			if t.ContainerTable.GetCell(prevRow, 0).GetReference() != nil {
				t.ContainerTable.Select(prevRow, 0)
				t.ActiveRow = prevRow
				break
			}
			prevRow--
		}
	} else {
		nextRow := t.ActiveRow + 1
		for nextRow < t.ContainerTable.GetRowCount() {
			if t.ContainerTable.GetCell(nextRow, 0).GetReference() != nil {
				t.ContainerTable.Select(nextRow, 0)
				t.ActiveRow = nextRow
				break
			}
			nextRow++
		}
	}
}
