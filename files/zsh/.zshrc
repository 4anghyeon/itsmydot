# itsmydot이 ~/.config/zsh 에 연결한 파일을 이 순서대로 불러온다.
for name in env oh-my-zsh mise go git killport android; do
    [[ -r ~/.config/zsh/$name.zsh ]] && source ~/.config/zsh/$name.zsh
done
unset name

# 토큰처럼 리포에 올리면 안 되는 설정은 Mac마다 이 파일에 따로 둔다.
[[ -r ~/.zshrc.local ]] && source ~/.zshrc.local
