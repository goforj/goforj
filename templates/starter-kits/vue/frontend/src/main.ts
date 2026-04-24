import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import 'vue-sonner/style.css'
import './style.css'
import { applyTheme, watchSystemTheme } from './lib/theme'

applyTheme()
watchSystemTheme()

const app = createApp(App)
app.use(router)
app.mount('#app')
