package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/brewfile"
	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/homebrew"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// brewItem 은 brewfiles 목록의 한 줄이다. 프로필 줄은 profile만 채우고 패키지 줄은 pkg와 프로필 이름인 parent를 채운다.
type brewItem struct {
	profile manifest.Brewfile
	pkg     brewfile.Package
	parent  string
}

func (it brewItem) nodeName() string {
	if it.parent == "" {
		return it.profile.Name
	}
	return it.pkg.Name
}

func (it brewItem) parentName() string { return it.parent }

// isProfile 은 프로필 줄이면 true다.
func (it brewItem) isProfile() bool { return it.parent == "" }

// description 은 커서가 이 줄에 있을 때 목록 아래에 보여 줄 설명이다.
func (it brewItem) description() string {
	if it.isProfile() {
		return it.profile.Description
	}
	return it.pkg.Description
}

// brewfileList 는 brewfiles 메뉴의 목록 화면이다. 프로필별 Homebrew 패키지와 설치 상태를 보여 주고 고른 패키지를 한 번에 설치한다.
type brewfileList struct {
	client  *github.Client
	find    func() (homebrew.Runner, error) // brew를 찾는다. 테스트에서는 가짜 Runner를 돌려준다
	runner  homebrew.Runner
	tempDir string // 설치할 줄만 모은 임시 Brewfile을 만들 곳. 비어 있으면 시스템 임시 폴더다

	version string
	width   int  // 터미널 폭. 모르면 0이다
	height  int  // 터미널 높이. 모르면 0이고 이때는 목록을 자르지 않고 모두 그린다
	light   bool // 터미널 배경이 밝으면 true

	items     []brewItem
	cursor    int
	offset    int             // 목록 창의 맨 위에 보이는 줄. 접힌 줄을 뺀 보이는 줄 기준이다
	collapsed map[string]bool // 접힌 프로필의 이름. 바꿀 때는 [withCollapsed] 로 새 map을 만든다
	installed homebrew.Installed

	// picked 는 설치하려고 고른 패키지의 [brewfile.Package.Key] 다. 바꿀 때는 새 map을 만든다.
	picked map[string]bool

	ready    bool  // Brewfile과 설치 상태를 다 불러왔으면 true
	missing  bool  // brew를 찾지 못했으면 true
	loadErr  error // Brewfile이나 설치 상태를 불러오지 못했을 때의 에러
	checking bool  // 설치가 끝난 뒤 설치 상태를 다시 확인하는 중이면 true

	confirming bool // 설치할지 묻는 중이면 true
	choice     int  // 확인 질문에서 커서가 있는 선택지

	// installing 은 마지막으로 설치를 시작한 패키지의 Key다. 설치가 끝나면 설치 상태를 다시 보고 실패한 수를 센다.
	installing []string

	statusLine
}

// newBrewfileList 는 처음 목록 상태를 만든다. 실제 불러오기는 [brewfileList.Init] 에서 시작한다.
func newBrewfileList(client *github.Client, version string) brewfileList {
	return brewfileList{client: client, find: findBrew, version: version}
}

// findBrew 는 [homebrew.Find] 를 Runner로 감싼다. 찾지 못했을 때 nil 포인터가 든 인터페이스를 돌려주지 않게 한다.
func findBrew() (homebrew.Runner, error) {
	b, err := homebrew.Find()
	if err != nil {
		return nil, err
	}
	return b, nil
}

// brewfilesLoadedMsg 는 brew를 찾고 Brewfile과 설치 상태를 불러온 결과다.
type brewfilesLoadedMsg struct {
	runner    homebrew.Runner
	items     []brewItem
	installed homebrew.Installed
	missing   bool
	err       error
}

// brewInstallFinishedMsg 는 터미널을 넘겨받은 brew bundle이 끝났다는 메시지다.
// 일부만 실패해도 brew가 에러로 끝나므로 결과는 설치 상태를 다시 확인해서 판단한다.
type brewInstallFinishedMsg struct{}

// brewInstalledCheckedMsg 는 설치가 끝난 뒤 설치 상태를 다시 확인한 결과다.
type brewInstalledCheckedMsg struct {
	installed homebrew.Installed
	err       error
}

// Init 은 brew를 찾고 Brewfile과 설치 상태 불러오기를 시작한다. 진입 메뉴에서 brewfiles를 처음 고를 때 한 번만 부른다.
func (m brewfileList) Init() tea.Cmd {
	client, find := m.client, m.find
	return func() tea.Msg {
		return loadBrewfiles(context.Background(), client, find)
	}
}

// loadBrewfiles 는 GitHub에서 manifest와 프로필별 Brewfile을 받고 brew로 설치 상태를 확인한다.
// Homebrew가 없으면 받지 않고 missing만 알린다.
func loadBrewfiles(ctx context.Context, client *github.Client, find func() (homebrew.Runner, error)) brewfilesLoadedMsg {
	runner, err := find()
	if errors.Is(err, homebrew.ErrNotInstalled) {
		return brewfilesLoadedMsg{missing: true}
	}
	if err != nil {
		return brewfilesLoadedMsg{err: err}
	}

	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return brewfilesLoadedMsg{err: err}
	}
	parsed, err := manifest.Parse(data)
	if err != nil {
		return brewfilesLoadedMsg{err: err}
	}
	var items []brewItem
	for _, profile := range parsed.Brewfiles {
		data, err := client.Fetch(ctx, profile.Source)
		if err != nil {
			return brewfilesLoadedMsg{err: err}
		}
		items = append(items, brewItem{profile: profile})
		for _, p := range brewfile.Parse(data) {
			items = append(items, brewItem{pkg: p, parent: profile.Name})
		}
	}

	installed, err := runner.Installed(ctx)
	if err != nil {
		return brewfilesLoadedMsg{err: err}
	}
	return brewfilesLoadedMsg{runner: runner, items: items, installed: installed}
}

// Update 는 불러오기와 설치 결과, 키 입력을 화면 상태에 반영한다.
func (m brewfileList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.offset = m.followCursor()
	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
	case brewfilesLoadedMsg:
		m.ready, m.missing, m.loadErr = true, msg.missing, msg.err
		m.runner, m.items, m.installed = msg.runner, msg.items, msg.installed
	case brewInstallFinishedMsg:
		return m.checkInstalled()
	case brewInstalledCheckedMsg:
		return m.applyInstalled(msg), nil
	case tea.KeyPressMsg:
		return m.handleKey(normalizeKey(msg))
	}
	return m, nil
}

// handleKey 는 목록과 확인 질문의 키 입력을 처리한다.
// 불러오는 중이거나 목록을 보여 줄 수 없을 때와 설치 상태를 다시 확인하는 중에는 나가는 키만 받는다.
func (m brewfileList) handleKey(key string) (brewfileList, tea.Cmd) {
	if m.confirming {
		return m.handleConfirmKey(key)
	}
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, backToMenu
	}
	if !m.ready || m.missing || m.loadErr != nil || len(m.items) == 0 || m.checking {
		return m, nil
	}

	m = m.countStatusKey()
	switch key {
	case "up", "k":
		m.cursor = treeNextVisible(m.items, m.collapsed, m.cursor, -1)
	case "down", "j":
		m.cursor = treeNextVisible(m.items, m.collapsed, m.cursor, 1)
	case "left", "h":
		m.cursor, m.collapsed = treeCollapse(m.items, m.collapsed, m.cursor)
	case "right", "l":
		m.collapsed = treeExpand(m.items, m.collapsed, m.cursor)
	case "space":
		m = m.togglePick()
	case "enter":
		if len(m.pickedPackages()) == 0 {
			m = m.notify(statusInfo, "설치할 패키지를 space로 먼저 골라 주세요")
		} else {
			m.confirming, m.choice = true, 0
		}
	}
	m.offset = m.followCursor()
	return m, nil
}

// handleConfirmKey 는 설치 확인 질문의 키 입력을 처리한다. 1번은 설치, 2번과 esc는 취소다.
func (m brewfileList) handleConfirmKey(key string) (brewfileList, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	options := brewConfirmOptions
	if key == "esc" {
		key = "2"
	}
	var picked int
	m.choice, picked = moveChoice(key, m.choice, len(options))
	if picked == 0 {
		return m, nil
	}
	m.confirming = false
	if picked == 2 {
		return m, nil
	}
	return m.startInstall()
}

// brewConfirmOptions 는 설치 확인 질문의 선택지다. 마지막은 취소다.
var brewConfirmOptions = []string{"설치", "취소"}

// togglePick 은 커서가 있는 패키지를 고르거나 고르기를 취소한다. 설치된 패키지는 고를 수 없다.
// 프로필 줄에서 누르면 설치 안 된 하위 패키지를 모두 고르고, 이미 모두 골라 두었으면 모두 취소한다.
func (m brewfileList) togglePick() brewfileList {
	it := m.items[m.cursor]
	var keys []string
	if it.isProfile() {
		for _, j := range treeChildren(m.items, m.cursor) {
			if p := m.items[j].pkg; !m.installed.Has(p) {
				keys = append(keys, p.Key())
			}
		}
		if len(keys) == 0 {
			return m.notify(statusInfo, it.profile.Name+" 패키지는 모두 설치돼 있어요")
		}
	} else {
		if m.installed.Has(it.pkg) {
			return m.notify(statusInfo, it.pkg.Name+" 이미 설치돼 있어요")
		}
		keys = []string{it.pkg.Key()}
	}

	allPicked := true
	for _, key := range keys {
		allPicked = allPicked && m.picked[key]
	}
	picked := make(map[string]bool, len(m.picked)+len(keys))
	for key := range m.picked {
		picked[key] = true
	}
	for _, key := range keys {
		if allPicked {
			delete(picked, key)
		} else {
			picked[key] = true
		}
	}
	m.picked = picked
	return m
}

// pickedPackages 는 고른 패키지를 목록 순서대로 한 번씩 모은다. 여러 프로필에 같은 패키지가 있어도 한 번만 넣는다.
func (m brewfileList) pickedPackages() []brewfile.Package {
	var packages []brewfile.Package
	seen := make(map[string]bool)
	for _, it := range m.items {
		key := it.pkg.Key()
		if it.isProfile() || !m.picked[key] || seen[key] {
			continue
		}
		seen[key] = true
		packages = append(packages, it.pkg)
	}
	return packages
}

// startInstall 은 고른 줄만 모은 임시 Brewfile을 만들고 brew bundle에 터미널을 넘긴다.
// 설치하는 동안 brew가 진행 상황을 직접 보여 주고 sudo 비밀번호도 brew 화면에서 받는다.
func (m brewfileList) startInstall() (brewfileList, tea.Cmd) {
	packages := m.pickedPackages()
	path, err := writeInstallBrewfile(m.tempDir, packages)
	if err != nil {
		return m.notify(statusFailed, "설치할 Brewfile을 만들지 못했어요 ("+errorReason(err, "")+")"), nil
	}
	m.installing = make([]string, len(packages))
	for i, p := range packages {
		m.installing[i] = p.Key()
	}
	m = m.notify(statusInfo, "")
	return m, tea.ExecProcess(m.runner.InstallCommand(path), func(error) tea.Msg {
		_ = os.Remove(path) // 지우지 못해도 임시 폴더에 남을 뿐이다
		return brewInstallFinishedMsg{}
	})
}

// writeInstallBrewfile 은 packages의 줄만 모은 임시 Brewfile을 dir에 쓰고 그 경로를 돌려준다.
// cask가 쓰는 tap이 먼저 등록되도록 tap 줄을 앞에 둔다.
func writeInstallBrewfile(dir string, packages []brewfile.Package) (string, error) {
	var taps, others []string
	for _, p := range packages {
		if p.Kind == brewfile.Tap {
			taps = append(taps, p.Line)
		} else {
			others = append(others, p.Line)
		}
	}
	f, err := os.CreateTemp(dir, "itsmydot-*.Brewfile")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(strings.Join(append(taps, others...), "\n") + "\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// checkInstalled 는 설치가 끝난 뒤 설치 상태를 백그라운드로 다시 확인한다.
func (m brewfileList) checkInstalled() (brewfileList, tea.Cmd) {
	m.checking = true
	m = m.notify(statusBusy, "설치 상태를 확인하는 중이에요...")
	runner := m.runner
	return m, func() tea.Msg {
		installed, err := runner.Installed(context.Background())
		return brewInstalledCheckedMsg{installed: installed, err: err}
	}
}

// applyInstalled 는 다시 확인한 설치 상태를 반영하고 이번에 설치하지 못한 패키지 수를 상태 줄에 알린다.
// 설치된 패키지는 고르기에서 빼고 실패한 패키지는 고른 채로 남겨 두어 바로 다시 설치해 볼 수 있게 한다.
func (m brewfileList) applyInstalled(msg brewInstalledCheckedMsg) brewfileList {
	m.checking = false
	if msg.err != nil {
		return m.notify(statusFailed, "설치 상태를 확인하지 못했어요 ("+errorReason(msg.err, "")+")")
	}
	m.installed = msg.installed

	picked := make(map[string]bool)
	for _, it := range m.items {
		if key := it.pkg.Key(); !it.isProfile() && m.picked[key] && !m.installed.Has(it.pkg) {
			picked[key] = true
		}
	}
	failed := 0
	for _, key := range m.installing {
		if picked[key] {
			failed++
		}
	}
	total := len(m.installing)
	m.picked, m.installing = picked, nil

	switch {
	case failed == 0:
		return m.notify(statusDone, fmt.Sprintf("%d개 패키지를 설치했어요", total))
	case failed == total:
		return m.notify(statusFailed, fmt.Sprintf("%d개 패키지를 설치하지 못했어요. 고른 채로 남겨 뒀어요", total))
	default:
		return m.notify(statusFailed, fmt.Sprintf("%d개 중 %d개 패키지를 설치하지 못했어요. 실패한 패키지는 고른 채로 남겨 뒀어요", total, failed))
	}
}

// installedCount 는 목록에 있는 패키지 중 설치된 것의 수와 전체 수다. 여러 프로필에 있는 같은 패키지는 한 번만 센다.
func (m brewfileList) installedCount() (installed, total int) {
	seen := make(map[string]bool)
	for _, it := range m.items {
		key := it.pkg.Key()
		if it.isProfile() || seen[key] {
			continue
		}
		seen[key] = true
		total++
		if m.installed.Has(it.pkg) {
			installed++
		}
	}
	return installed, total
}

// notify 는 상태 줄에 text를 띄운 목록 모델을 돌려준다. text가 비어 있으면 상태 줄을 비운다.
func (m brewfileList) notify(tone statusTone, text string) brewfileList {
	m.statusLine = newStatusLine(tone, text)
	return m
}

// countStatusKey 는 목록에서 누른 키를 상태 줄에 센 목록 모델을 돌려준다.
func (m brewfileList) countStatusKey() brewfileList {
	m.statusLine = m.statusLine.countKey()
	return m
}
