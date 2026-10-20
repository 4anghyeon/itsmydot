package tui

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"os"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// errorReason 은 에러를 화면에 보여 줄 사유 문장으로 바꾼다. 원래 에러 내용은 항목 메뉴에서 따로 보여 준다.
// home은 권한 에러의 경로를 ~로 줄이는 데 쓴다.
func errorReason(err error, home string) string {
	var rateLimit *github.RateLimitError
	var invalid *manifest.InvalidError
	var netErr net.Error
	var urlErr *url.Error
	switch {
	case errors.Is(err, github.ErrNotFound):
		return "저장소에 파일이 없어요"
	case errors.As(err, &rateLimit):
		if rateLimit.Reset.IsZero() {
			return "GitHub 요청 한도를 다 썼어요. 잠시 뒤에 다시 해 주세요"
		}
		return "GitHub 요청 한도를 다 썼어요. " + rateLimit.Reset.Local().Format("15:04") + " 이후에 다시 해 주세요"
	case errors.As(err, &invalid):
		return "manifest.yaml 형식이 잘못됐어요"
	// http.Client의 Timeout이 지나면 url.Error로 감싼 timeout 에러가 오므로 연결 실패보다 먼저 본다.
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "GitHub가 응답하지 않아요. 잠시 뒤에 다시 해 주세요"
	case errors.As(err, &urlErr):
		return "GitHub에 접속하지 못했어요. 네트워크를 확인해 주세요"
	case errors.Is(err, fs.ErrPermission):
		if path := errorPath(err); path != "" {
			return tildify(path, home) + " 파일에 접근할 권한이 없어요"
		}
		return "파일에 접근할 권한이 없어요"
	default:
		return "예상하지 못한 문제가 생겼어요"
	}
}

// errorPath 는 파일 에러가 가리키는 경로다. symlink를 만들다 난 에러는 새로 만들려던 쪽 경로를 쓴다.
func errorPath(err error) string {
	var pathErr *fs.PathError
	var linkErr *os.LinkError
	switch {
	case errors.As(err, &pathErr):
		return pathErr.Path
	case errors.As(err, &linkErr):
		return linkErr.New
	}
	return ""
}
