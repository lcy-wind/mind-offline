<script setup lang="ts">
import {ref,watch,onMounted} from 'vue';import {paintBeads} from '../lib/beads';
const props=withDefaults(defineProps<{pixels:number[];size:number;round?:boolean}>(),{round:true});const canvas=ref<HTMLCanvasElement|null>(null);
function draw(){const c=canvas.value;if(!c)return;c.width=c.height=144;const ctx=c.getContext('2d')!;ctx.fillStyle='#f6f6eb';ctx.fillRect(0,0,144,144);paintBeads(ctx,props.pixels,props.size,144/props.size,props.round);}
onMounted(draw);watch(()=>[props.pixels,props.size,props.round],draw,{deep:true});
</script><template><component :is="'canvas'" ref="canvas" class="bead-thumbnail" aria-label="拼豆图案预览"/></template><style scoped>.bead-thumbnail{display:block;width:100%;height:auto;aspect-ratio:1;border-radius:14px}</style>
