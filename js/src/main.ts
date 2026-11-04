import "./app.css";
import { mount } from "svelte";
import App from "./App.svelte";

const legacyTerm = document.getElementById("terminal");
let target = document.getElementById("app");

if (!target) {
    target = document.createElement("div");
    target.id = "app";
    if (legacyTerm && legacyTerm.parentNode) {
        legacyTerm.parentNode.replaceChild(target, legacyTerm);
    } else {
        document.body.appendChild(target);
    }
}

mount(App, { target });
