agcb() {
  local types=(
    "1) feat:      new feature"
    "2) fix:       bug fix"
    "3) refactor:  refactoring"
    "4) style:     style change"
    "5) docs:      documentation"
    "6) chore:     etc"
  )

  echo "Select branch type:"
  for t in "${types[@]}"; do
    echo "  $t"
  done

  echo -n "Enter number (1-6) > "
  read -r num

  case $num in
    1) type="feat" ;;
    2) type="fix" ;;
    3) type="refactor" ;;
    4) type="style" ;;
    5) type="docs" ;;
    6) type="chore" ;;
    *) echo "Invalid number" && return 1 ;;
  esac

  echo -n "Enter branch name (ex: add-icon-button) > "
  read -r branch_name

  [[ -z "$branch_name" ]] && echo "No branch name, cancelled" && return 1

  local full_branch="lsh/$type/$branch_name"
  echo "Created: $full_branch"
  git checkout -b "$full_branch"
}

# 로컬 브랜치를 fzf로 골라 switch
gswb() {
  if ! command -v fzf >/dev/null; then
    echo "fzf가 필요합니다. brew install fzf 로 설치하세요"
    return 1
  fi

  local branch
  branch=$(git branch --format='%(refname:short)' | fzf --height=40% --reverse --preview 'git log --oneline -10 {}') || return
  git switch "$branch"
}
