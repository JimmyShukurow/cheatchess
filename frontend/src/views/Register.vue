<template>
  <div class="auth-page">
    <div class="card auth-card">
      <div class="head">
        <img src="/pieces/cburnett/wN.svg" alt="" />
        <h1>Join now</h1>
        <p class="muted">Create an account and play in seconds.</p>
      </div>
      <template v-if="state.googleEnabled">
        <GoogleButton label="Sign up with Google" />
        <div class="divider">OR WITH EMAIL</div>
      </template>
      <form @submit.prevent="submit">
        <label>Email</label>
        <input v-model="email" class="input" type="email" autocomplete="email" required />
        <label>Username</label>
        <input v-model="username" class="input" autocomplete="username" required minlength="3" maxlength="20" pattern="[A-Za-z0-9_\-]+" title="Letters, digits, _ or -" />
        <label>Password</label>
        <input v-model="password" class="input" type="password" autocomplete="new-password" required minlength="6" />
        <div v-if="error" class="error">{{ error }}</div>
        <button class="btn btn-green btn-block submit" :disabled="busy">Sign Up</button>
      </form>
      <p class="muted foot">Already have an account? <router-link to="/login" class="link">Log in</router-link></p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { state, register } from '../store.js'
import GoogleButton from '../components/GoogleButton.vue'

const router = useRouter()
const email = ref('')
const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await register(email.value, username.value, password.value)
    router.push('/')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>
