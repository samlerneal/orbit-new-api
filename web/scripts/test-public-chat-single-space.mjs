/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { spawnSync } from 'node:child_process'

const bunExecutable = process.execPath
const testGroups = [
  [
    'test',
    '--preload',
    './src/features/playground/public-chat-dom.test-setup.ts',
    './src/features/playground/public-chat-single-space.integration.test.tsx',
  ],
  ['test', './src/features/playground/public-chat.test.tsx'],
  ['test', './src/features/playground/lib/public-chat-route.test.ts'],
  [
    'test',
    './src/features/playground/hooks/use-public-chat-state.test.ts',
    './src/features/playground/hooks/use-stream-request.test.ts',
    './src/features/playground/lib/public-chat-storage.test.ts',
    './src/features/playground/lib/public-chat-sessions.test.ts',
    './src/features/playground/components/input/public-chat-input.test.tsx',
    './src/features/playground/components/public-chat-sidebar.test.tsx',
    './src/i18n/locales/locales.test.ts',
  ],
]

const forbiddenOutput = [
  /not wrapped in act/i,
  /act\(\.\.\.\) warning/i,
  /The current testing environment is not configured to support act/i,
  /(?:^|\n)\s*[1-9]\d* skip\b/i,
  /test environment.*warning/i,
]

const representativeReactEnvironmentWarning =
  'The current testing environment is not configured to support act(...)'
if (
  !forbiddenOutput.some((pattern) =>
    pattern.test(representativeReactEnvironmentWarning)
  )
) {
  console.error('Runner does not reject the representative React warning')
  process.exit(1)
}

for (const args of testGroups) {
  const result = spawnSync(bunExecutable, args, {
    cwd: process.cwd(),
    encoding: 'utf8',
    env: { ...process.env, NODE_ENV: 'test' },
  })
  const output = `${result.stdout ?? ''}${result.stderr ?? ''}`
  process.stdout.write(output)

  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
  const warning = forbiddenOutput.find((pattern) => pattern.test(output))
  if (warning) {
    console.error(`Forbidden test output matched: ${warning}`)
    process.exit(1)
  }
}
