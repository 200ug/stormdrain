package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	notifTTL = 15 * time.Second

	defaultTextColor       = tcell.ColorWhite
	inactiveColor          = tcell.ColorGray
	headingColor           = tcell.ColorBlanchedAlmond
	modalBgColor           = tcell.ColorDarkCyan
	modalBorderColor       = tcell.ColorBlack
	titleColor             = tcell.ColorBlanchedAlmond
	toolNameColor          = tcell.ColorOrange
	selectionColor         = tcell.ColorLightBlue
	notificationColor      = tcell.ColorLightGreen
	errorNotificationColor = tcell.ColorIndianRed
)

type TUI struct {
	VersionCode      string
	ActiveRow        int
	UserHome         string
	DataManager      *manager.Manager
	Profiles         []*manager.Profile
	App              *tview.Application
	Pages            *tview.Pages
	HeaderView       *tview.TextView
	NotificationView *tview.TextView
	ContainerTable   *tview.Table
	DetailView       *tview.TextView
	notifSetAt       time.Time
}

func NewTUI(m *manager.Manager, versionCode string) *TUI {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil // needs to be checkd in caller to avoid panics
	}

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
	notificationView.SetBorderPadding(0, 0, 1, 0)

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
		SetTitle(" Details ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(headerView, 3, 0, false).
		AddItem(notificationView, 1, 0, false).
		AddItem(containerTable, 0, 1, true).
		AddItem(detailView, 8, 0, false)

	pages := tview.NewPages().AddPage("main", flex, true, true)

	app.SetRoot(pages, true).EnableMouse(false)

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
		case 'n':
			createView, form := tui.newCreateView()
			if createView == nil {
				tui.showErrror("Error: could not initialize container creation view")
				return nil
			}
			tui.Pages.AddPage("create", createView, true, false)
			tui.Pages.SwitchToPage("create")
			tui.App.SetFocus(form)
			return nil
		case 's':
			container := tui.getSelectedContainer()
			if container == nil {
				tui.showErrror("Error: no container selected")
				return nil
			}
			spec, err := manager.LoadSpec(container.ProjectPath, container.Name)
			if err != nil {
				tui.showErrror(fmt.Sprintf("Error: could not load spec: %s", err))
				return nil
			}
			tui.DataManager.CmdChan <- manager.Command{Type: manager.Stop, Spec: *spec, Force: false}
			tui.showNotification("Stopping container...", true)
			return nil
		case 'x':
			// identical to stopping, but with Force = true (i.e. kill)
			container := tui.getSelectedContainer()
			if container == nil {
				tui.showErrror("Error: no container selected")
				return nil
			}
			spec, err := manager.LoadSpec(container.ProjectPath, container.Name)
			if err != nil {
				tui.showErrror(fmt.Sprintf("Error: could not load spec: %s", err))
				return nil
			}
			tui.DataManager.CmdChan <- manager.Command{Type: manager.Stop, Spec: *spec, Force: true}
			tui.showNotification("Killing container...", true)
			return nil
		case 'd':
			container := tui.getSelectedContainer()
			if container == nil {
				tui.showErrror("Error: no container selected")
				return nil
			}
			modal := tui.newRemoveConfirmModal(container.Name, container)
			tui.Pages.AddPage("confirm_remove", modal, true, false)
			tui.Pages.SwitchToPage("confirm_remove")
			return nil
		case 'p':
			modal := tui.newPurgeConfirmModal()
			tui.Pages.AddPage("confirm_purge", modal, true, false)
			tui.Pages.SwitchToPage("confirm_purge")
			return nil
		case 'a':
			// NOTE: bypasses CmdChan entirely because AttachIntoContainer needs direct terminal access
			container := tui.getSelectedContainer()
			if container == nil {
				tui.showErrror("Error: no container selected")
				return nil
			}
			spec, err := manager.LoadSpec(container.ProjectPath, container.Name)
			if err != nil {
				tui.showErrror(fmt.Sprintf("Error: could not load spec: %s", err))
				return nil
			}
			tui.App.Suspend(func() { // blocks here until we detach from the container session
				spec.AttachIntoContainer()
			})
			tui.showNotification("Restored previous state successfully", false)
			tui.updateContainerTable()
			tui.updateDetails()
			return nil
		default:
			return event
		}
	})

	tui = &TUI{
		VersionCode:      versionCode,
		ActiveRow:        0, // header row, i.e. no selection
		UserHome:         userHome,
		DataManager:      m,
		Profiles:         make([]*manager.Profile, 0),
		App:              app,
		Pages:            pages,
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
				t.handleNotifications()
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

func (t *TUI) handleNotifications() {
	if !t.notifSetAt.IsZero() && time.Since(t.notifSetAt) > notifTTL {
		t.cleanNotification()
	}
	select {
	case msg := <-t.DataManager.NotifChan:
		t.showNotification(msg, false)
	case err := <-t.DataManager.ErrChan:
		t.showErrror(fmt.Sprintf("Error: %s", err))
	default:
	}
}

func (t *TUI) getSelectedContainer() *manager.Container {
	if t.ActiveRow < 1 || t.ActiveRow >= t.ContainerTable.GetRowCount() {
		return nil
	}
	ref := t.ContainerTable.GetCell(t.ActiveRow, 0).GetReference()
	if ref == nil {
		return nil
	}
	name, ok := ref.(string)
	if !ok {
		return nil
	}
	t.DataManager.Mu.RLock()
	defer t.DataManager.Mu.RUnlock()
	for _, c := range t.DataManager.Containers {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

func (t *TUI) updateDetails() {
	container := t.getSelectedContainer()
	if container == nil {
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

	headers := []string{"Name", "Profile", "Status", "CPU (dir/avg)", "Memory"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetSelectable(false).
			SetTextColor(headingColor).
			SetExpansion(1)
		t.ContainerTable.SetCell(0, i, cell)
	}

	for i, c := range containers {
		data := []string{c.Name, c.ProfileName, c.StatusString(), c.CPU, c.Memory}
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

func (t *TUI) newCreateView() (*tview.Flex, *tview.Form) {
	t.cleanNotification()

	t.collectProfiles()
	if len(t.Profiles) == 0 {
		return nil, nil
	}
	profileNames := make([]string, len(t.Profiles))
	for i, p := range t.Profiles {
		profileNames[i] = p.Name
	}

	// defaults that also track the state depending on the selected profile
	defaultProjectPath, err := os.Getwd()
	if err != nil {
		return nil, nil
	}
	defaultShell := manager.DefaultShell
	defaultProjectMount := false

	var selectedProfile *manager.Profile
	if len(t.Profiles) > 0 {
		selectedProfile = t.Profiles[0]
		if selectedProfile.Shell != "" {
			defaultShell = selectedProfile.Shell
		}
		if selectedProfile.ProjectMount != nil {
			defaultProjectMount = *selectedProfile.ProjectMount
		}
	}

	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(" Create Container ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(titleColor)
	form.SetFieldBackgroundColor(modalBgColor)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(titleColor)
	form.SetButtonBackgroundColor(tcell.ColorDarkCyan)
	form.SetButtonTextColor(tcell.ColorWhite)
	form.AddDropDown("Profile", profileNames, 0, func(option string, idx int) {
		if idx < len(t.Profiles) {
			selectedProfile = t.Profiles[idx]
		}
	})
	form.AddInputField("Project path", defaultProjectPath, 50, nil, nil)
	form.AddCheckbox("Project mount", defaultProjectMount, nil)
	form.AddInputField("Shell", defaultShell, 20, nil, nil)

	errView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { t.App.Draw() })
	errView.SetTextColor(errorNotificationColor)

	returnToMain := func() {
		errView.SetText("")
		t.Pages.SwitchToPage("main")
		t.App.SetFocus(t.ContainerTable)
	}

	form.AddButton("Create", func() {
		errView.SetText("")

		if selectedProfile == nil {
			errView.SetText("no profile selected").SetTextColor(errorNotificationColor)
			return
		}

		// 1. resolve project path
		projectPath := form.GetFormItem(1).(*tview.InputField).GetText()
		if projectPath == "" {
			projectPath = "."
		}
		absProjectPath, err := filepath.Abs(projectPath)
		if err != nil {
			errView.SetText(fmt.Sprintf("invalid project path: %s", err)).SetTextColor(errorNotificationColor)
			return
		}

		// validate: path must exist and be a directory
		info, err := os.Stat(absProjectPath)
		if err != nil {
			errView.SetText(fmt.Sprintf("project path does not exist: %s", absProjectPath)).SetTextColor(errorNotificationColor)
			return
		}
		if !info.IsDir() {
			errView.SetText(fmt.Sprintf("project path is not a directory: %s", absProjectPath)).SetTextColor(errorNotificationColor)
			return
		}

		// create container name first for directory scoping
		containerName, hostname := manager.UniqueContainerName(filepath.Base(absProjectPath))

		// 2. substitute profile values to dockerfile template
		configsDir := filepath.Join(t.UserHome, ".config", "stormdrain")
		if err := selectedProfile.SubstituteDockerfileTemplate(configsDir, absProjectPath, containerName); err != nil {
			errView.SetText(fmt.Sprintf("Dockerfile substitution failed: %s", err)).SetTextColor(errorNotificationColor)
			return
		}

		// 3. stage configs temporarily to .stormdrain/configs
		if err := selectedProfile.StageConfigs(t.UserHome, absProjectPath, containerName); err != nil {
			errView.SetText(fmt.Sprintf("Config staging failed: %s", err)).SetTextColor(errorNotificationColor)
			return
		}

		// 4. create spec profile (with previously generated containerName/hostname) and apply potential overrides
		spec, err := manager.NewSpecWithContainerName(selectedProfile, absProjectPath, containerName, hostname)
		if err != nil {
			errView.SetText(fmt.Sprintf("Could not create spec: %s", err)).SetTextColor(errorNotificationColor)
			return
		}
		spec.ProjectMount = form.GetFormItem(2).(*tview.Checkbox).IsChecked()
		if shell := form.GetFormItem(3).(*tview.InputField).GetText(); shell != "" {
			spec.Shell = shell
		}

		// 5. send create command to backend manager (handles CreateContainer + WriteToDisk + CleanupStagedConfigs)
		t.DataManager.CmdChan <- manager.Command{Type: manager.Create, Spec: *spec}
		t.showNotification(fmt.Sprintf("Creating container '%s'...", hostname), true)

		t.Pages.SwitchToPage("main")
		t.App.SetFocus(t.ContainerTable)
	})

	form.AddButton("Cancel", returnToMain)

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			returnToMain()
			return nil
		}
		return event
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(errView, 1, 0, false)

	return flex, form
}

func (t *TUI) collectProfiles() {
	configsDir := filepath.Join(t.UserHome, ".config", "stormdrain")
	profilesDir := filepath.Join(configsDir, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return
	}
	// NOTE: filter out unrelated results, and skip i/o caused by reading the full file
	// if no apparent changes have been made
	var profileEntries []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			profileEntries = append(profileEntries, entry)
		}
	}
	if len(profileEntries) == len(t.Profiles) {
		return
	}

	var profiles []*manager.Profile
	for _, profileEntry := range profileEntries {
		profile, err := manager.LoadProfile(configsDir, strings.TrimSuffix(profileEntry.Name(), ".json"))
		if err != nil {
			continue // this could just be random json file in the wrong place, no need to fail
		}
		profiles = append(profiles, profile)
	}
	t.Profiles = profiles
}

func (t *TUI) newRemoveConfirmModal(containerName string, container *manager.Container) *tview.Modal {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Remove container %s?\nThis will also delete its image and container data under .stormdrain/ directory.", containerName)).
		AddButtons([]string{"Remove", "Cancel"}).
		SetTextColor(defaultTextColor).
		SetBackgroundColor(modalBgColor)
	modal.SetBorderColor(modalBorderColor)
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonIndex == 0 { // "Remove"
			spec, err := manager.LoadSpec(container.ProjectPath, containerName)
			if err != nil {
				t.showErrror(fmt.Sprintf("Error: could not load spec: %s", err))
				t.Pages.SwitchToPage("main")
				t.App.SetFocus(t.ContainerTable)
				return
			}
			t.DataManager.CmdChan <- manager.Command{Type: manager.Remove, Spec: *spec}
			t.showNotification("Removing container...", true)
		}
		t.Pages.SwitchToPage("main")
		t.App.SetFocus(t.ContainerTable)
	})
	return modal
}

func (t *TUI) newPurgeConfirmModal() *tview.Modal {
	t.DataManager.Mu.RLock()
	count := len(t.DataManager.Containers)
	t.DataManager.Mu.RUnlock()

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Purge all %d container(s)?\nThis will stop, remove, and delete all containers, their images, volumes, and .stormdrain/ directories. This cannot be undone.", count)).
		AddButtons([]string{"Purge", "Cancel"}).
		SetTextColor(defaultTextColor).
		SetBackgroundColor(modalBgColor)
	modal.SetBorderColor(modalBorderColor)
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonIndex == 0 { // "Purge"
			t.DataManager.CmdChan <- manager.Command{Type: manager.Purge}
			t.showNotification("Purging all containers, images, and volumes...", true)
		}
		t.Pages.SwitchToPage("main")
		t.App.SetFocus(t.ContainerTable)
	})
	return modal
}

func (t *TUI) showNotification(text string, skipClearing bool) {
	t.NotificationView.SetText(text).SetTextColor(notificationColor)
	if !skipClearing {
		t.notifSetAt = time.Now()
	}
}

func (t *TUI) cleanNotification() {
	t.NotificationView.SetText("")
	t.notifSetAt = time.Time{}
}

func (t *TUI) showErrror(text string) {
	t.NotificationView.SetText(text).SetTextColor(errorNotificationColor)
	t.notifSetAt = time.Now()
}
