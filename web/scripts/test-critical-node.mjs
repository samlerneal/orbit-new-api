/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { createRequire } from 'node:module'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webDirectory = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..'
)
const requireFromWeb = createRequire(path.join(webDirectory, 'package.json'))
const { createRsbuild } = requireFromWeb('@rsbuild/core')
const temporaryDirectory = await mkdtemp(
  path.join(os.tmpdir(), 'orbit-critical-node-test-')
)

const testEntries = [
  ['root-auth-bootstrap', 'src/routes/__tests__/root-auth-bootstrap.test.tsx'],
  [
    'authenticated-navigation',
    'src/components/layout/components/__tests__/authenticated-navigation.test.tsx',
  ],
]

function runNodeTest(bundlePath) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ['--test', bundlePath], {
      cwd: webDirectory,
      env: {
        ...process.env,
        NODE_ENV: 'test',
        NODE_PATH: [
          path.join(webDirectory, 'node_modules'),
          process.env.NODE_PATH,
        ]
          .filter(Boolean)
          .join(path.delimiter),
      },
      stdio: 'inherit',
    })
    child.once('error', reject)
    child.once('exit', (code, signal) => {
      if (signal) {
        reject(new Error(`node:test ended from signal ${signal}`))
        return
      }
      if (code === 0) {
        resolve()
        return
      }
      reject(new Error(`node:test exited with code ${code ?? 1}`))
    })
  })
}

try {
  for (const [name, relativeEntry] of testEntries) {
    const rsbuild = await createRsbuild({
      cwd: webDirectory,
      rsbuildConfig: {
        source: { entry: { [name]: path.join(webDirectory, relativeEntry) } },
        resolve: {
          alias: { '@': path.join(webDirectory, 'src') },
          conditionNames: ['browser', 'import', 'module', 'default'],
        },
        output: {
          target: 'node',
          module: false,
          distPath: { root: temporaryDirectory, js: '.' },
          filename: { js: '[name].cjs' },
          externals: {
            react: 'commonjs react',
            'react-dom': 'commonjs react-dom',
            'react-dom/client': 'commonjs react-dom/client',
            'react/jsx-runtime': 'commonjs react/jsx-runtime',
            'react/jsx-dev-runtime': 'commonjs react/jsx-dev-runtime',
          },
          autoExternal: false,
          cleanDistPath: false,
        },
        performance: { buildCache: false },
        tools: {
          swc: {
            jsc: { transform: { react: { runtime: 'automatic' } } },
          },
        },
      },
    })
    await rsbuild.build()
    await runNodeTest(path.join(temporaryDirectory, `${name}.cjs`))
  }
} finally {
  await rm(temporaryDirectory, { recursive: true, force: true })
}
