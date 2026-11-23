# ~/.zshrc 맨 아래의 한 줄이 이 파일을 불러온다.
# itsmydot이 ~/.config/zsh 에 연결한 파일을 이 순서대로 불러온다.
for name in env oh-my-zsh fzf mise go git killport android; do
    [[ -r ~/.config/zsh/$name.zsh ]] && source ~/.config/zsh/$name.zsh
done
unset name
