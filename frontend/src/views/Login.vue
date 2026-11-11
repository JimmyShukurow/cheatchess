<template>
  <div class="auth-page">
    <div class="card auth-card">
      <div class="head">
        <img src="/pieces/cburnett/wN.svg" alt="" />
        <h1>Log in</h1>
      </div>
      <form @submit.prevent="submit">
        <label>Email or username</label>
        <input v-model="loginName" class="input" autocomplete="username" required autofocus />
        <label>Password</label>
        <input v-model="password" class="input" type="password" autocomplete="current-password" required />
        <div v-if="error" class="error">{{ error }}</div>
        <button class="btn btn-green btn-block submit" :disabled="busy">Log In</button>
      </form>
      <template v-if="state.googleEnabled">
        <div class="divider">OR</div>
        <GoogleButton />
      </template>
      <p class="muted foot">New here? <router-link to="/register" class="link">Sign up – and start playing</router-link></p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { state, login } from '../store.js'
import GoogleButton from '../components/GoogleButton.vue'

const route = useRoute()
const router = useRouter()
const loginName = ref('')
const password = ref('')
const busy = ref(false)
const googleErrors = { state: 'Google login expired, please try again.', exchange: 'Google login failed.', userinfo: 'Could not read your Google profile.', account: 'Could not create the account.', session: 'Login failed.' }
const error = ref(googleErrors[route.query.error] || '')

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await login(loginName.value, password.value)
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : '/'
    router.push(next)
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<style>
.auth-page { min-height: calc(100vh - 20px); display: grid; place-items: center; padding: 24px; }
.auth-card { width: 100%; max-width: 400px; padding: 32px; }
.auth-card .head { text-align: center; }
.auth-card .head img { width: 64px; }
.auth-card h1 { margin: 4px 0 12px; font-size: 28px; }
.auth-card .submit { margin-top: 22px; padding: 14px; font-size: 18px; }
.auth-card .foot { text-align: center; margin: 22px 0 0; font-size: 14px; }
.auth-card .link { color: var(--green); font-weight: 700; }
</style>
