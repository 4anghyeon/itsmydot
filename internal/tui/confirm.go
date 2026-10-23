package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// confirmKind 는 어떤 확인 질문을 띄웠는지 나타낸다.
type confirmKind int

const (
	confirmBackupLink          confirmKind = iota // 다른 파일이 있어서 백업한 뒤 연결할지 묻는 중
	confirmUnlinkWithoutBackup                    // 되돌릴 백업이 없어서 연결을 어떻게 끊을지 묻는 중
	confirmGroupUnlink                            // 켜진 하위 항목이 있어서 그룹 파일과 함께 끌지 묻는 중
	confirmAction                                 // enter로 연 항목 메뉴에서 할 일을 고르는 중
	confirmTurnOn                                 // 다른 질문 없이 켜지는 항목을 정말 켤지 묻는 중
	confirmTurnOff                                // 다른 질문 없이 꺼지는 항목을 정말 끌지 묻는 중
)

// confirmPrompt 는 확인 질문 문구와 선택지를 만든다. handleConfirmKey가 마지막 선택지를 취소로 보므로 순서를 지켜야 한다.
func (m Model) confirmPrompt() (question string, options []string) {
	i := m.indexOf(m.confirmingName)
	if i < 0 {
		return "", []string{"취소"}
	}
	it := m.items[i]
	target := tildify(it.entry.Target, m.home)

	// 켜진 하위 항목이 있으면 그룹 파일을 끌 때 함께 꺼진다는 걸 미리 알린다.
	childrenNote := ""
	if children := m.syncedChildren(i); len(children) > 0 {
		names := make([]string, len(children))
		for k, j := range children {
			names[k] = m.items[j].entry.Name
		}
		childrenNote = fmt.Sprintf("하위 항목 %d개(%s)도 함께 연결이 끊겨요. ", len(children), strings.Join(names, ", "))
	}

	switch m.confirmKind {
	case confirmUnlinkWithoutBackup:
		return fmt.Sprintf("%s 백업이 없어서 되돌릴 원본 파일이 없어요. %s연결을 어떻게 끊을까요?", target, childrenNote),
			[]string{"지금 설정은 남겨 두고 연결만 끊기 (추천)", "연결을 끊고 파일도 지우기", "취소"}
	case confirmAction:
		question := it.entry.Name + "  " + target
		if it.state == synced {
			return question, []string{"최신으로 받기", "차이 보기", "연결 끊기", "닫기"}
		}
		return question, []string{"연결", "닫기"}
	case confirmGroupUnlink:
		return fmt.Sprintf("%s 연결을 끊으면 %s계속할까요?", it.entry.Name, childrenNote),
			[]string{"모두 연결 끊기", "취소"}
	case confirmTurnOff:
		return fmt.Sprintf("%s  %s 연결을 끊을까요? 백업해 둔 원본 파일을 되돌려요.", it.entry.Name, target),
			[]string{"연결 끊기", "취소"}
	case confirmTurnOn:
		question := fmt.Sprintf("%s  %s 파일을 연결할까요?", it.entry.Name, target)
		if it.parent != "" && slices.Contains(m.pendingNames, it.parent) {
			parent := m.items[m.indexOf(it.parent)]
			question += fmt.Sprintf(" 그룹 파일 %s  %s 도 함께 연결해요.", parent.entry.Name, tildify(parent.entry.Target, m.home))
		}
		return question, []string{"연결", "취소"}
	default:
		var conflicted []string
		for _, name := range m.pendingNames {
			if j := m.indexOf(name); j >= 0 && m.items[j].state == conflict {
				conflicted = append(conflicted, tildify(m.items[j].entry.Target, m.home))
			}
		}
		question := fmt.Sprintf("%s 에 이미 파일이 있어요. 기존 파일을 백업한 뒤 연결할까요?", strings.Join(conflicted, ", "))
		if it.parent != "" && slices.Contains(m.pendingNames, it.parent) {
			question = fmt.Sprintf("그룹 파일 %s 도 함께 연결해요. ", it.parent) + question
		}
		return question, []string{"백업 후 연결", "차이 보기", "취소"}
	}
}

// handleConfirmKey 는 확인 질문에 대한 키 입력을 처리한다. 방향키와 enter나 space로 고르거나 번호로 바로 고르고 esc는 취소다.
// 그 밖의 키는 무시해서 답하기 전에 다른 항목을 건드리지 않게 한다.
func (m Model) handleConfirmKey(key string) (Model, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	_, options := m.confirmPrompt()
	cancel := strconv.Itoa(len(options)) // 마지막 선택지는 항상 취소다
	switch key {
	case "up", "k":
		m.choice = max(m.choice-1, 0)
		return m, nil
	case "down", "j":
		m.choice = min(m.choice+1, len(options)-1)
		return m, nil
	case "enter", "space":
		key = strconv.Itoa(m.choice + 1)
	case "esc":
		key = cancel
	}
	picked, err := strconv.Atoi(key)
	if err != nil || picked < 1 || picked > len(options) {
		return m, nil
	}

	// 차이 보기는 답이 아니라 답을 고르려고 보는 것이라 질문을 닫지 않는다.
	if m.confirmKind == confirmBackupLink && picked == 2 {
		return m.startTargetCompare()
	}

	name, pending := m.confirmingName, m.pendingNames
	m.confirmingName, m.pendingNames, m.confirmNote = "", nil, ""
	if m.confirmKind == confirmBackupLink {
		// 질문에서 연 비교는 질문이 닫히면 보여 줄 곳이 없으니 늦게 온 결과를 버린다.
		m.comparingName = ""
	}
	if key == cancel {
		// 질문 전에 떠 있던 알림은 지금과 상관없어서 함께 지운다.
		m = m.notify(statusInfo, "")
		return m, nil
	}

	i := m.indexOf(name)
	if i < 0 {
		return m, nil
	}
	m.items = slices.Clone(m.items)

	switch m.confirmKind {
	case confirmBackupLink, confirmTurnOn:
		indexes := make([]int, 0, len(pending))
		for _, p := range pending {
			if j := m.indexOf(p); j >= 0 {
				indexes = append(indexes, j)
			}
		}
		return m.startSync(indexes...)
	case confirmUnlinkWithoutBackup:
		if picked == 1 { // 1. 지금 설정은 남겨 두고 연결만 끊기
			return m.detach(i).startAnimation()
		}
		// 2. 연결을 끊고 파일도 지우기
		return m.unlink(i).startAnimation()
	case confirmGroupUnlink, confirmTurnOff:
		return m.unlink(i).startAnimation()
	case confirmAction:
		// 메뉴를 여는 동안 커서는 움직이지 않으므로 커서가 곧 메뉴를 연 항목이다.
		if m.items[i].state == synced {
			switch picked {
			case 1: // 최신으로 받기
				return m.updateCopy(i)
			case 2: // 차이 보기
				return m.startCompare(i)
			}
		}
		return m.toggle() // 연결 또는 연결 끊기
	}
	return m, nil
}

// openMenu 는 커서가 있는 항목에서 할 수 있는 일을 번호 메뉴로 띄운다. 받는 중인 항목에서는 열지 않는다.
func (m Model) openMenu() Model {
	if !m.ready || len(m.items) == 0 || m.items[m.cursor].state == syncing {
		return m
	}
	m.confirmingName, m.confirmKind, m.choice = m.items[m.cursor].entry.Name, confirmAction, 0
	return m
}

// renderConfirmLines 는 확인 질문이나 메뉴를 질문, 번호 선택지, 진행 상황 순으로 한 줄씩 만든다.
// 선택지 번호도 함께 돌려줘서 도움말 줄에 쓸 수 있게 한다.
func (m Model) renderConfirmLines(st styles) (lines, numbers []string) {
	question, options := m.confirmPrompt()
	title := st.question.Render(question)
	if m.confirmKind == confirmAction {
		title = m.renderMenuTitle(st)
		// 사유 문장만으로는 원인을 알기 어려울 때를 위해 원래 에러를 메뉴에서만 보여 준다.
		if i := m.indexOf(m.confirmingName); i >= 0 && m.items[i].state == errored && m.items[i].err != nil {
			title += "\n" + st.muted.Render(m.items[i].err.Error())
		}
	}
	for line := range strings.SplitSeq(title, "\n") {
		lines = append(lines, " "+line)
	}
	lines = append(lines, "")
	numbers = make([]string, len(options))
	for i, opt := range options {
		numbers[i] = strconv.Itoa(i + 1)
		cursor, label := " ", opt
		if i == m.choice {
			cursor, label = st.cursor.Render(">"), st.selected.Render(opt)
		}
		lines = append(lines, " "+cursor+" "+st.key.Render(numbers[i]+".")+" "+label)
	}
	switch {
	case m.comparingName != "":
		lines = append(lines, "", " "+st.stateStyle(syncing).Render(m.renderMarker(syncing))+" "+m.confirmNote)
	case m.confirmNote != "":
		lines = append(lines, "", " "+m.confirmNote)
	}
	return lines, numbers
}

// renderMenuTitle 은 항목 메뉴의 제목이다.
func (m Model) renderMenuTitle(st styles) string {
	return m.renderItemTitle(st, m.indexOf(m.confirmingName))
}
