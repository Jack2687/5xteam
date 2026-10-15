import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { ref, reactive, computed } from 'vue'
import { createLatestRequest } from '../src/latestRequest.js'

const tick = () => new Promise(resolve => setImmediate(resolve))
const source = readFileSync(new URL('../src/components/ProRestockView.vue', import.meta.url), 'utf8')
const code = source.split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm, '')
function fixture() {
  const requests = [], timers = new Map()
  let mounted, unmounted, timerID = 0
  const context = vm.createContext({
    ref, reactive, computed, createLatestRequest, nextTick: tick,
    onMounted(fn) { mounted = fn }, onUnmounted(fn) { unmounted = fn },
    setInterval(fn) { const id = ++timerID; timers.set(id, fn); return id },
    clearInterval(id) { timers.delete(id) },
    api(url, options = {}) {
      let resolve, reject
      const promise = new Promise((ok, fail) => { resolve = ok; reject = fail })
      requests.push({ url, options, resolve, reject })
      return promise
    },
  })
  vm.runInContext(code + '\nglobalThis.exposed = {state, form, records, page, refresh, pause, evaluate, resetBreaker, loadRecords, error, collectionError, notice, monitorUnavailable};', context)
  return { requests, timers, mounted: () => mounted(), unmounted: () => unmounted(), ...context.exposed }
}

// An initial request completing after navigation must not resurrect polling.
{
  const f = fixture()
  const mount = f.mounted()
  await tick()
  f.unmounted()
  f.requests[0].resolve({ config: { enabled: true, version: 1 } })
  await tick()
  for (const r of f.requests.slice(1)) r.resolve([])
  await mount
  assert.equal(f.timers.size, 0, 'unmounted page recreated polling')
  assert.equal(f.requests.length, 1, 'unmounted page started another request')
  assert.equal(f.state.value, null, 'unmounted page accepted an old response')
}

// A late GET cannot undo the state returned by a pause/evaluate/reset action.
for (const action of ['pause', 'evaluate', 'resetBreaker']) {
  const f = fixture()
  f.state.value = { config: { enabled: true, version: 1 } }
  const oldPoll = f.refresh()
  await tick()
  const old = f.requests[0]
  const operation = f[action]()
  await tick()
  const mutation = f.requests.find(r => r.options.method)
  assert.ok(mutation, 'missing action request')
  const config = { enabled: false, version: 2 }
  mutation.resolve(action === 'pause' ? config : { config })
  await tick()
  for (const r of f.requests) {
    if (r === old || r === mutation) continue
    r.resolve(r.url.includes('/records') ? [] : { config })
  }
  await tick()
  for (const r of f.requests.filter(r => r.url.includes('/records'))) r.resolve([])
  await operation
  old.resolve({ config: { enabled: true, version: 1 } })
  await tick()
  for (const r of f.requests.filter(r => r.url.includes('/records'))) r.resolve([])
  await oldPoll
  assert.equal(f.state.value.config.version, 2, `${action}: old response replaced new state`)
  if (action === 'pause') assert.equal(f.form.version, 2, 'pause did not refresh settings form')
  f.unmounted()
}

// Paging cannot be overwritten by an older page's records response.
{
  const f = fixture()
  const first = f.loadRecords()
  await tick()
  f.page.value = 2
  const second = f.loadRecords()
  await tick()
  f.requests[1].resolve([{ id: 'page-2' }])
  await second
  f.requests[0].resolve([{ id: 'page-1' }])
  await first
  assert.equal(f.records.value[0].id, 'page-2', 'old page replaced current records')
  f.unmounted()
}
// Manual retry reports the new collection result and preserves the saved mode.
for (const scenario of ['failed', 'load_failed', 'stale', 'auto', 'observe', 'disabled', 'busy']) {
  const f = fixture()
  f.state.value = { config: { enabled: true, mode: 'auto' }, snapshot: { valid: false } }
  assert.equal(f.monitorUnavailable.value, true)
  const operation = f.evaluate()
  await tick()
  const mutation = f.requests[0]
  assert.equal(mutation.url, '/api/pro-restock/evaluate')
  if (scenario === 'busy') {
    mutation.reject(new Error('正在采集或更新补货配置，请稍后重试'))
  } else {
    mutation.resolve({
      config: { enabled: scenario !== 'disabled', mode: scenario === 'observe' ? 'observe' : 'auto', fresh_seconds: 20 },
      snapshot: { valid: scenario !== 'failed', load_valid: scenario !== 'load_failed', at: new Date(Date.now() - (scenario === 'stale' ? 60000 : 0)).toISOString(), error: scenario === 'failed' ? '接口超时' : '' },
    })
    await tick()
    for (const r of f.requests.filter(r => r.url.includes('/records'))) r.resolve([])
  }
  await operation
  if (['failed', 'load_failed', 'stale', 'busy'].includes(scenario)) {
    assert.ok(f.error.value || f.collectionError.value, `${scenario}: missing failure feedback`)
    assert.equal(f.notice.value, '', `${scenario}: false success feedback`)
    assert.equal(f.monitorUnavailable.value, true)
  } else {
    assert.equal(f.error.value, '')
    assert.equal(f.monitorUnavailable.value, false)
    assert.match(f.notice.value, scenario === 'auto' ? /恢复规则判断/ : scenario === 'observe' ? /观察模式不会/ : /手动暂停/)
    if (scenario === 'disabled') assert.equal(f.state.value.config.enabled, false)
  }
  if (scenario === 'failed') {
    const poll = f.refresh(false, true)
    await tick()
    f.requests.findLast(r => r.url === '/api/pro-restock').resolve({ config: {enabled: true, mode: 'auto', fresh_seconds: 20}, snapshot: {valid: true, load_valid: true, at: new Date().toISOString()} })
    await tick()
    f.requests.findLast(r => r.url.includes('/records')).resolve([])
    await poll
    assert.equal(f.collectionError.value, '', 'automatic recovery left old manual retry failure visible')
    assert.equal(f.monitorUnavailable.value, false)
  }
  f.unmounted()
}
console.log('Passed: restock request ordering, record paging, and manual retry failure/recovery feedback')
