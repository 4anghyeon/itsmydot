# Android Studio가 SDK를 설치하는 기본 위치. 설치돼 있을 때만 쓴다.
if [[ -d $HOME/Library/Android/sdk ]]; then
  export ANDROID_HOME="$HOME/Library/Android/sdk"
  export ANDROID_SDK_ROOT="$ANDROID_HOME"
  export PATH="$PATH:$ANDROID_HOME/platform-tools:$ANDROID_HOME/cmdline-tools/latest/bin"
fi
