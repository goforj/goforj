import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { useDevconsoleStore } from "./stores/devconsole";
import "vue-sonner/style.css";
import "./style.css";
import "./style-goforj.css";
import "./style-glass.css"

/* Toggle one GoForj variant at a time: */
// import "./style-goforj-variant-crisp.css";
// import "./style-goforj-variant-soft.css";
// import "./style-goforj-variant-night.css";



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
