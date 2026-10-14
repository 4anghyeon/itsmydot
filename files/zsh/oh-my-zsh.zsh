export ZSH="$HOME/.oh-my-zsh"
if [[ -r $ZSH/oh-my-zsh.sh ]]; then
  ZSH_THEME="robbyrussell"
  plugins=(git)
  source $ZSH/oh-my-zsh.sh
fi
