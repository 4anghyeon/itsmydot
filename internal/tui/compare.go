package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/diff"
)

// startCompare 는 i번째 항목의 로컬 사본과 비교할 저장소 최신본을 백그라운드로 받는다.
// 업데이트 표시에 쓰는 해시만으로는 내용을 알 수 없어서 비교할 때마다 새로 받는다.
func (m Model) startCompare(i int) (Model, tea.Cmd) {
	it := m.items[i]
	m.comparingName = it.entry.Name
	m = m.notify(statusBusy, it.entry.Name+" 저장소 최신본과 비교하는 중이에요...")
	entry, copyPath, client := it.entry, it.copyPath, m.client
	load := func() tea.Msg {
		return compareWithRepo(context.Background(), client, entry, copyPath)
	}
	return m, tea.Batch(load, m.spinner.Tick)
}

// startTargetCompare 는 충돌 질문에서 target 자리에 있는 지금 파일과 저장소 파일을 비교한다. 질문은 그대로 둔다.
// 그룹 파일과 하위 파일이 함께 걸렸으면 질문을 띄운 항목을 보고, 그 항목이 충돌이 아니면 처음 걸린 항목을 본다.
func (m Model) startTargetCompare() (Model, tea.Cmd) {
	i := m.conflictedIndex()
	if i < 0 {
		return m, nil
	}
	it := m.items[i]
	m.comparingName = it.entry.Name
	m.confirmNote = it.entry.Name + " 저장소 파일과 비교하는 중이에요..."
	entry, client := it.entry, m.client
	load := func() tea.Msg {
		msg := compareWithRepo(context.Background(), client, entry, entry.Target)
		msg.current = true
		return msg
	}
	return m, tea.Batch(load, m.spinner.Tick)
}

// conflictedIndex 는 충돌 질문에서 비교할 항목의 위치다. 걸린 항목이 없으면 -1이다.
func (m Model) conflictedIndex() int {
	if i := m.indexOf(m.confirmingName); i >= 0 && m.items[i].state == conflict {
		return i
	}
	for _, name := range m.pendingNames {
		if j := m.indexOf(name); j >= 0 && m.items[j].state == conflict {
			return j
		}
	}
	return -1
}

// applyDiff 는 비교 결과로 비교 보기 화면을 연다. 차이가 없거나 비교할 수 없으면 화면 대신 한 줄로 알린다.
// 메뉴에서 연 비교는 상태 메시지로, 충돌 질문에서 연 비교는 질문 아래에 알린다.
// 받는 사이에 다른 비교를 시작했거나 질문이 닫혔으면 늦게 온 결과는 버린다.
func (m Model) applyDiff(msg diffLoadedMsg) Model {
	if msg.name != m.comparingName {
		return m
	}
	m.comparingName = ""

	same := msg.name + " 사본과 저장소 최신본이 같아요"
	if msg.current {
		same = msg.name + " 지금 파일과 저장소 파일이 같아요"
	}
	var note string
	tone := statusInfo
	switch {
	case errors.Is(msg.err, diff.ErrBinary):
		note = msg.name + " 파일은 텍스트가 아니라서 비교할 수 없어요"
	case errors.Is(msg.err, diff.ErrTooLarge):
		note = fmt.Sprintf("%s 파일이 %dMB보다 커서 비교할 수 없어요", msg.name, diff.MaxSize>>20)
	case msg.err != nil:
		note, tone = msg.name+" 파일을 비교하지 못했어요 ("+errorReason(msg.err, m.home)+")", statusFailed
	case len(msg.hunks) == 0:
		note = same
	default:
		m.diffScreen = diffScreen{name: msg.name, rows: diffRows(msg.hunks)}
	}

	if msg.current {
		m.confirmNote = note
	} else {
		m = m.notify(tone, note)
	}
	return m
}

// diffRows 는 hunk를 화면 줄로 펼친다. hunk마다 머리줄을 먼저 놓고 줄바꿈이 없는 줄 아래에는 안내 줄을 붙인다.
func diffRows(hunks []diff.Hunk) []diffRow {
	var rows []diffRow
	for _, hunk := range hunks {
		rows = append(rows, diffRow{kind: hunkRow, header: hunk.Header()})
		for _, line := range hunk.Lines {
			rows = append(rows, diffRow{kind: lineRow, line: line})
			if line.NoNewline {
				rows = append(rows, diffRow{kind: noNewlineRow})
			}
		}
	}
	return rows
}
