import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { useDevconsoleStore } from "./stores/devconsole";
import "vue-sonner/style.css";
import "./style.css";

// import "./style-dark-blue.css";
import "./style-vitepress.css";
import "./style-discord.css";


(async () => {
  const store = useDevconsoleStore();
  await store.bootstrap();
  if (!store.state.authenticated) {
    router.replace("/login");
  } else {
    store.connectSocket();
  }
  const app = createApp(App);
  app.use(router);
  await router.isReady();
  app.mount("#app");
})();
