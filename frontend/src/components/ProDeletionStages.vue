<script setup>
import { computed } from 'vue'
import { LoaderCircle } from 'lucide-vue-next'
import { formatTime } from '../utils'
const props = defineProps({ account: { type: Object, required: true }, disabled: Boolean })
defineEmits(['retry'])
const state = computed(() => props.account.pro_deactivation || {})
const steps = [['credentials', '凭据'], ['verify', '验证'], ['submit', '提交'], ['confirm', '确认']]
const names = { running: '执行中', completed: '成功', waiting: '等待确认', failed: '失败', unknown: '结果待核验', interrupted: '已中断' }
const groups = computed(() => props.account.pro_post_merge_groups)
</script>
<template>
  <div class="deletion-flow">
    <div class="deletion-steps"><span v-for="[key,label] in steps" :key="key" :class="state.steps?.[key]"><LoaderCircle v-if="state.steps?.[key] === 'running'" class="spin" :size="11"/><b>{{ label }}</b>{{ names[state.steps?.[key]] || '未开始' }}</span></div>
    <small v-if="state.message">{{ state.message }}</small>
    <small v-if="state.accepted_at">提交成功：{{ formatTime(state.accepted_at) }}</small>
    <small v-else-if="state.submitted_at">提交请求：{{ formatTime(state.submitted_at) }}</small>
    <small v-if="state.confirmed_at">确认时间：{{ formatTime(state.confirmed_at) }}</small>
    <small v-if="state.next_check_at">{{ state.confirmed_at ? '下次后续处理' : '下次核验' }}：{{ formatTime(state.next_check_at) }}</small>
    <small v-if="state.error" class="danger-text">{{ state.error }}</small>
    <small v-if="groups">分组调整：{{ names[groups.status] || '待执行' }}<span v-if="groups.error" class="danger-text"> · {{ groups.error }}</span></small>
    <button v-if="state.submitted_at && (state.error || groups && groups.status !== 'completed')" type="button" class="btn ghost compact" :disabled="disabled" @click="$emit('retry')">{{ state.confirmed_at ? '续跑分组调整' : '续跑核验及分组调整' }}</button>
  </div>
</template>
<style scoped>
.deletion-flow{min-width:280px;display:grid;gap:5px}.deletion-steps{display:grid;grid-template-columns:repeat(2,1fr);gap:4px}.deletion-steps>span{display:flex;align-items:center;gap:5px;padding:4px 6px;border:1px solid var(--line);border-radius:5px;font-size:11px;color:var(--muted)}.deletion-flow small{font-size:11px;color:var(--muted);overflow-wrap:anywhere}.deletion-steps .completed{color:var(--green-strong);background:var(--green-bg)}.deletion-steps .running,.deletion-steps .waiting,.deletion-steps .unknown{color:var(--amber);background:var(--amber-bg)}.deletion-steps .failed{color:var(--red);background:var(--red-bg)}.spin{animation:spin 1s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
</style>
