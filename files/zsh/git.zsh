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
