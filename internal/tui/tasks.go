package tui

import (
	"context"
	"os"
	"path"

	"github.com/4anghyeon/itsmydot/internal/diff"
	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/link"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
	"github.com/4anghyeon/itsmydot/internal/rcfile"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// entriesLoadedMsg 는 manifest를 불러와 각 항목의 상태까지 확인한 결과다.
type entriesLoadedMsg struct {
	items  []item
	hashes map[string]string // 저장소 파일의 blob 해시. 받지 못했으면 nil이다
	err    error
}

// entrySyncedMsg 는 syncEntry나 fetchCopy가 항목 하나를 받아 온 결과다.
type entrySyncedMsg struct {
	name        string
	backup      string // 기존 파일을 옮겨 둔 백업 경로. 백업하지 않았으면 빈 문자열
	lineAdded   bool   // loaded_by 파일에 불러오는 줄을 새로 넣었으면 true
	copyUpdated bool   // 연결은 그대로 두고 로컬 사본만 새로 받았으면 true
	err         error
}

// diffLoadedMsg 는 저장소 최신본과 비교한 결과다. 두 파일이 같으면 hunks가 비어 있다.
type diffLoadedMsg struct {
	name    string
	hunks   []diff.Hunk
	err     error
	current bool // 로컬 사본 대신 target 자리의 지금 파일과 비교했으면 true. 충돌 질문에서 연 비교다
}

// previewLoadedMsg 는 미리보기 창에 보여 줄 저장소 파일을 받아 온 결과다. key는 캐시에 넣을 정리한 source 경로다.
type previewLoadedMsg struct {
	name string
	key  string
	data []byte
	err  error
}

// loadEntries 는 GitHub에서 manifest를 받아 파싱하고 각 항목의 현재 상태를 확인한다. 디스크는 바꾸지 않는다.
func loadEntries(ctx context.Context, client *github.Client, copies *localcopy.Dir, home string) entriesLoadedMsg {
	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return entriesLoadedMsg{err: err}
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return entriesLoadedMsg{err: err}
	}

	hashes, _ := client.FileHashes(ctx)

	// 하위 항목은 그룹 바로 뒤에 이어 붙여서 목록 순서가 화면 순서와 같게 한다.
	var items []item
	for _, e := range m.Entries {
		items = append(items, checkItem(e, "", copies, hashes, home))
		for _, child := range e.Children {
			items = append(items, checkItem(child, e.Name, copies, hashes, home))
		}
	}
	return entriesLoadedMsg{items: items, hashes: hashes}
}

// checkItem 은 항목 하나의 로컬 사본 경로와 지금 상태를 확인해 목록의 한 줄을 만든다.
// 확인에 실패해도 나머지 항목은 보여 주도록 그 항목만 ui.Errored로 표시한다.
func checkItem(e manifest.Entry, parent string, copies *localcopy.Dir, hashes map[string]string, home string) item {
	e.Children = nil
	it := item{entry: e, parent: parent}
	var err error
	it.copyPath, err = copies.Path(e.Source)
	if err == nil {
		var s link.State
		s, err = checkLink(e, it.copyPath, home)
		it.state = fromLinkState(s)
	}
	if err != nil {
		it.state, it.err = ui.Errored, err
	}
	if it.state == ui.Synced {
		it.outdated = differsFromRepo(it.copyPath, e.Source, hashes)
	}
	return it
}

// checkLink 는 target 자리를 보고 항목의 연결 상태를 판단한다. 디스크는 바꾸지 않는다.
// loaded_by가 있는 항목은 symlink가 걸려 있어도 그 파일에 불러오는 줄이 없으면 연결되지 않은 것으로 본다.
// 그래야 ~/.zshrc를 통째로 바꾸는 도구가 줄을 지웠을 때 다시 켜서 줄을 넣을 수 있다.
func checkLink(e manifest.Entry, copyPath, home string) (link.State, error) {
	state, err := link.Check(e.Target, copyPath)
	if err != nil || state != link.Synced || e.LoadedBy == "" {
		return state, err
	}
	has, err := rcfile.Has(e.LoadedBy, rcfile.Line(e.Target, home))
	if err != nil {
		return 0, err
	}
	if !has {
		return link.NotSynced, nil
	}
	return link.Synced, nil
}

// linkEntry 는 target에 로컬 사본을 연결하고 loaded_by가 있으면 그 파일 끝에 불러오는 줄을 넣는다.
func linkEntry(e manifest.Entry, copyPath, home string) (backup string, lineAdded bool, err error) {
	backup, err = link.Link(e.Target, copyPath)
	if err != nil || e.LoadedBy == "" {
		return backup, false, err
	}
	lineAdded, err = rcfile.Add(e.LoadedBy, rcfile.Line(e.Target, home))
	return backup, lineAdded, err
}

// removeLoadLine 은 loaded_by 파일에서 target을 불러오는 줄을 지운다. loaded_by가 없으면 아무것도 하지 않는다.
func removeLoadLine(e manifest.Entry, home string) (bool, error) {
	if e.LoadedBy == "" {
		return false, nil
	}
	return rcfile.Remove(e.LoadedBy, rcfile.Line(e.Target, home))
}

// differsFromRepo 는 로컬 사본이 저장소의 source 파일과 다른지 알려 준다.
// 저장소 해시가 없거나 사본을 읽지 못하면 비교할 수 없으므로 다르지 않다고 본다.
func differsFromRepo(copyPath, source string, hashes map[string]string) bool {
	remote, ok := hashes[path.Clean(source)]
	if !ok {
		return false
	}
	data, err := os.ReadFile(copyPath)
	if err != nil {
		return false
	}
	return github.BlobHash(data) != remote
}

// syncEntry 는 항목 하나를 GitHub에서 최신본으로 받아 로컬 사본에 저장하고 target에 연결한다.
// 이미 연결된 항목이면 로컬 사본만 최신으로 바뀐다.
func syncEntry(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry, home string) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name}

	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	copyPath, err := copies.Write(e.Source, data)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.backup, msg.lineAdded, msg.err = linkEntry(e, copyPath, home)
	return msg
}

// fetchCopy 는 연결된 항목의 최신본을 받아 로컬 사본만 바꾼다. symlink는 사본을 가리키고 있으므로 바로 반영된다.
func fetchCopy(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name, copyUpdated: true}
	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	_, msg.err = copies.Write(e.Source, data)
	return msg
}

// fromLinkState 는 link 패키지의 디스크 상태를 화면 상태로 바꾼다.
func fromLinkState(s link.State) ui.ItemState {
	switch s {
	case link.Synced:
		return ui.Synced
	case link.Conflict:
		return ui.Conflict
	default:
		return ui.NotSynced
	}
}

// compareWithRepo 는 저장소 최신본을 받아 path의 파일과 비교한다. 디스크는 바꾸지 않는다.
func compareWithRepo(ctx context.Context, client *github.Client, e manifest.Entry, path string) diffLoadedMsg {
	msg := diffLoadedMsg{name: e.Name}
	remote, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	local, err := os.ReadFile(path)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.hunks, msg.err = diff.Compare(local, remote)
	return msg
}
