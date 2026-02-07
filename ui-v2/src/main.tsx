import { render } from "solid-js/web";
import { AppRouter } from "./app/router";
import "./design/tokens/index.css";

render(() => <AppRouter />, document.getElementById("root")!);
