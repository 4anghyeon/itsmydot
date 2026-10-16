---
paths:
  - "**/*.{ts,tsx,js,jsx,mjs,cjs}"
---

# TypeScript / JavaScript

## Style

- 삼항 연산자를 중첩하지 않는다. 조건이 두 단계 이상이면 early return 하는 헬퍼 함수나 if 문으로 푼다.

## Types

- `any`를 최대한 지양한다. 타입을 모르면 `unknown`으로 받고 좁혀서 쓴다.
- `enum` 대신 `as const` 객체나 문자열 유니온을 쓴다.
- 타입 단언(`as`)과 non-null 단언(`!`)은 피하고 타입 가드로 좁힌다.