killport() {
  local port="$1"

  if [[ -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    echo "usage: killport <port>"
    return 1
  fi

  local pids
  pids=$(lsof -ti tcp:"$port")

  if [[ -z "$pids" ]]; then
    echo "포트 $port 사용 중인 프로세스 없음"
    return 0
  fi

  # 뭘 죽이는지 먼저 보여주기
  echo "포트 $port 사용 중:"
  lsof -i tcp:"$port" -sTCP:LISTEN

  # 1단계: SIGTERM — 프로세스가 스스로 정리하고 종료
  echo "$pids" | xargs kill 2>/dev/null

  # 최대 5초 대기하며 종료 확인
  local i
  for i in {1..10}; do
    sleep 0.5
    pids=$(lsof -ti tcp:"$port")
    [[ -z "$pids" ]] && { echo "정상 종료됨"; return 0; }
  done

  # 2단계: 그래도 살아있으면 SIGKILL
  echo "응답 없음 → 강제 종료(SIGKILL)"
  echo "$pids" | xargs kill -9 2>/dev/null
  echo "포트 $port 정리 완료"
}
