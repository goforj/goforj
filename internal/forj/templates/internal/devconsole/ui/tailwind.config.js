/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,vue}"],
  theme: {
    extend: {
      colors: {
        surface: "#0c0d10",
        panel: "#151821",
        border: "#242836",
        accent: "#8c97e6",
        accentStrong: "#5b6fe1",
        muted: "#9aa3b2",
      },
    },
  },
  plugins: [],
};
