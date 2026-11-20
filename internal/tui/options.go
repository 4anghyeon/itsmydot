package tui

import "strconv"

// 진입 메뉴와 확인 질문은 같은 번호 선택지를 쓴다.
// macOS 한글 입력기는 자모 하나를 다음 키가 올 때까지 붙잡아 두므로 y/n 대신 입력기가 붙잡지 않는 숫자와 방향키로 고르게 한다.

// moveChoice 는 번호 선택지에서 누른 키를 처리하고 옮긴 커서 위치와 고른 번호를 돌려준다.
// enter와 space는 커서가 있는 선택지를 고르고 숫자는 그 번호를 바로 고른다. 고르지 않았으면 picked는 0이다.
func moveChoice(key string, choice, count int) (next, picked int) {
	switch key {
	case "up", "k":
		return max(choice-1, 0), 0
	case "down", "j":
		return min(choice+1, count-1), 0
	case "enter", "space":
		return choice, choice + 1
	}
	number, err := strconv.Atoi(key)
	if err != nil || number < 1 || number > count {
		return choice, 0
	}
	return choice, number
}

// renderOptions 는 선택지를 번호와 함께 한 줄씩 그리고 커서가 있는 줄 앞에 > 를 붙인다.
// 선택지 번호도 함께 돌려줘서 도움말 줄에 쓸 수 있게 한다.
func (st styles) renderOptions(options []string, choice int) (lines, numbers []string) {
	numbers = make([]string, len(options))
	for i, option := range options {
		numbers[i] = strconv.Itoa(i + 1)
		cursor, label := " ", option
		if i == choice {
			cursor, label = st.cursor.Render(">"), st.selected.Render(option)
		}
		lines = append(lines, " "+cursor+" "+st.key.Render(numbers[i]+".")+" "+label)
	}
	return lines, numbers
}
