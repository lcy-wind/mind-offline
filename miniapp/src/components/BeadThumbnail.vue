<script setup lang="ts">
import {ref,watch,onMounted} from 'vue';import {paintBeads} from '../lib/beads';
const props=withDefaults(defineProps<{pixels:number[];size:number;height?:number;palette?:string;round?:boolean}>(),{round:true});const canvas=ref<HTMLCanvasElement|null>(null);
function draw(){const c=canvas.value;if(!c)return;const h=props.height||props.size;if(!Number.isFinite(props.size)||props.size<1||props.size>128||!Number.isFinite(h)||h<1||h>128){c.width=c.height=1;return;}c.width=144;c.height=Math.max(1,Math.round(144*h/props.size));const ctx=c.getContext('2d')!;ctx.fillStyle='#f6f6eb';ctx.fillRect(0,0,c.width,c.height);paintBeads(ctx,props.pixels,props.size,144/props.size,props.round,false,props.palette,h);}
onMounted(draw);watch(()=>[props.pixels,props.size,props.height,props.palette,props.round],draw,{deep:true});
</script><template><component :is="'canvas'" ref="canvas" class="bead-thumbnail" :style="{aspectRatio:size+'/'+(height||size)}" aria-label="拼豆图案预览"/></template><style scoped>.bead-thumbnail{display:block;width:100%;height:auto;max-height:240px;object-fit:contain;border-radius:14px}</style>
