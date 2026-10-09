<script setup lang="ts">
import { onLaunch } from "@dcloudio/uni-app";
onLaunch(() => {});
</script>
<style>
page {
  background: #f4f1e8;
  color: #23271f;
  font-family:
    -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", sans-serif;
  font-size: 14px;
}
view,
text,
button,
input,
textarea {
  box-sizing: border-box;
}
button {
  margin: 0;
  padding: 0;
  background: none;
  font-size: inherit;
  line-height: inherit;
  color: inherit;
  border-radius: 0;
}
button::after {
  border: 0;
}
button[disabled] {
  opacity: 0.45;
}
::-webkit-scrollbar {
  width: 5px;
  height: 3px;
}
::-webkit-scrollbar-thumb {
  background: #cccbbf;
  border-radius: 5px;
}
</style>
