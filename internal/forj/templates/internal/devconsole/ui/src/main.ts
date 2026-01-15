import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { useDevconsoleStore } from "./stores/devconsole";
import "./styles.css";

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
