<template>
  <div class="auth-page">
    <div class="card auth-card">
      <div class="head">
        <h1>{{ hasPassword ? 'Change password' : 'Set a password' }}</h1>
        <p v-if="!hasPassword" class="muted small">
          You signed up with Google. Set a password to also log in with your email or username.
        </p>
      </div>
      <form @submit.prevent="submit">
        <input :value="state.user?.username" autocomplete="username" hidden readonly />
        <template v-if="hasPassword">
          <label>Current password</label>
          <input v-model="current" class="input" type="password" autocomplete="current-password" required autofocus />
        </template>
        <label>New password</label>
        <input v-model="next" class="input" type="password" autocomplete="new-password" minlength="6" maxlength="72" required />
        <label>Confirm new password</label>
        <input v-model="confirm" class="input" type="password" autocomplete="new-password" required />
        <div v-if="error" class="error">{{ error }}</div>
        <div v-if="done" class="success">Password saved. Other devices have been logged out.</div>
        <button class="btn btn-green btn-block submit" :disabled="busy">Save password</button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { state, changePassword } from '../store.js'

const hasPassword = computed(() => !!state.user?.hasPassword)
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref('')
const done = ref(false)
const busy = ref(false)

async function submit() {
  error.value = ''
  done.value = false
  if (next.value !== confirm.value) {
    error.value = 'New passwords do not match'
    return
  }
  busy.value = true
  try {
    await changePassword(current.value, next.value)
    current.value = next.value = confirm.value = ''
    done.value = true
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.small { font-size: 14px; margin: 0 0 6px; }
.success { background: rgba(129, 182, 76, .15); color: #a8d67a; padding: 10px 12px; border-radius: 6px; font-size: 14px; margin-top: 12px; }
</style>
