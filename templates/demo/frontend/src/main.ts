import { createApp } from 'vue'
import './style.css'
import 'vue-sonner/style.css'
import App from './App.vue'
import router from './router'

document.documentElement.classList.add('dark')

createApp(App).use(router).mount('#app')
