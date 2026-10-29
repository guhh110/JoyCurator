<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMutation } from '@tanstack/vue-query'
import {
  ArrowRight,
  Film,
  Folder,
  Layers3,
  LoaderCircle,
  ShieldCheck,
} from '@lucide/vue'
import { api } from '../api'
import {
  session,
  acceptSession,
  safeRedirect,
} from '../app/session'
import { queryClient } from '../app/query'
const route = useRoute(),
  router = useRouter()
const initialized = computed(() => session.initialized)
const loginName = ref('admin'),
  password = ref(''),
  message = ref('')
const action = useMutation({
  mutationFn: () =>
    api<{ username: string }>(initialized.value ? '/login' : '/setup', {
      username: loginName.value,
      password: password.value,
    }),
})
const busy = action.isPending
async function login() {
  if (busy.value) return
  message.value = ''
  try {
    const value = await action.mutateAsync()
    password.value = ''
    queryClient.clear()
    acceptSession({ initialized: true, username: value.username })
    await router.replace(safeRedirect(route.query.redirect))
  } catch (error) {
    message.value = (error as Error).message
  }
}
</script>
<template>
  <div class="login-screen">
    <div class="login-art">
      <div class="brand light">
        <div class="brand-symbol"><Layers3 :size="25" /></div>
        JoyCurator<span class="edition">轻量版</span>
      </div>
      <div class="login-copy">
        <span class="eyebrow">A PLACE FOR EVERY STORY</span>
        <h1>好故事，<br />也值得好好收藏。</h1>
        <p>让散落的媒体文件，<br />成为井然有序的私人片库。</p>
        <div class="art-shelves">
          <div><Film /><span>电影</span><i>FILMS</i></div>
          <div><Layers3 /><span>剧集</span><i>SERIES</i></div>
          <div><Folder /><span>你的收藏</span><i>COLLECTION</i></div>
        </div>
      </div>
      <span class="art-caption">本地文件 · 手动配对 · 先预览再整理</span>
    </div>
    <main class="login-form">
      <div class="mobile-brand brand"><Layers3 />JoyCurator</div>
      <span class="eyebrow">WELCOME TO YOUR COLLECTION</span>
      <h2>{{ initialized ? '欢迎回来' : '创建你的工作台' }}</h2>
      <p class="muted">
        {{
          initialized
            ? '登录，继续整理你的收藏。'
            : '设置一个管理员账号，开始整理本地媒体。'
        }}
      </p>
      <form @submit.prevent="login">
        <label
          >用户名<input
            v-model="loginName"
            autocomplete="username"
            required
            maxlength="64" /></label
        ><label
          >密码<input
            v-model="password"
            type="password"
            :autocomplete="initialized ? 'current-password' : 'new-password'"
            :minlength="initialized ? 1 : 12"
            maxlength="72"
            required
            :placeholder="initialized ? '输入密码' : '至少 12 个字符'"
        /></label>
        <p v-if="message" role="alert" class="notice error">{{ message }}</p>
        <button class="button primary full" :disabled="busy">
          <LoaderCircle v-if="busy" class="spin" :size="17" />{{
            initialized ? '进入工作台' : '创建并进入'
          }}<ArrowRight :size="17" />
        </button>
      </form>
      <p class="login-note">
        <ShieldCheck :size="15" />文件与凭据保存在你的设备上
      </p>
    </main>
  </div>
</template>
