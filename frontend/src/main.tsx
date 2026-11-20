import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import { ConfirmProvider } from "@/components/ConfirmDialog";
import "./index.css";
import { PAGE_TITLE } from "@/brand";

document.title = PAGE_TITLE;

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfirmProvider>
      <App />
    </ConfirmProvider>
  </React.StrictMode>
);
