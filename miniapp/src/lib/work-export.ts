import {apiBase,currentToken,ApiError} from './api';
export async function downloadWorkExcel(path:string,filename='牛马工作记录.xlsx'):Promise<void>{
 const session=currentToken();
 // #ifdef H5
 const response=await fetch(apiBase()+'/api'+path,{headers:{Authorization:'Bearer '+session}});
 if(session!==currentToken())throw new ApiError('账号已切换，请重新导出',0);
 if(!response.ok){const data=await response.json().catch(()=>({}));throw new ApiError(data.error||'导出失败，请稍后重试',response.status);}
 const blob=await response.blob();if(session!==currentToken())throw new ApiError('账号已切换，请重新导出',0);
 const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download=filename;document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),30000);
 // #endif
 // #ifdef MP-WEIXIN
 await new Promise<void>((resolve,reject)=>{
  const discard=(filePath:string)=>{uni.getFileSystemManager().unlink({filePath,fail(){}});};
  uni.downloadFile({url:apiBase()+'/api'+path,header:{Authorization:'Bearer '+session},timeout:30000,success:r=>{
   if(session!==currentToken()){discard(r.tempFilePath);reject(new ApiError('账号已切换，请重新导出',0));return;}
   if(r.statusCode!==200){discard(r.tempFilePath);reject(new ApiError(r.statusCode===401?'登录已过期，请重新登录':'导出失败，请稍后重试',r.statusCode));return;}
   uni.openDocument({filePath:r.tempFilePath,fileType:'xlsx',showMenu:true,success:()=>resolve(),fail:()=>reject(new Error('文件已生成，但微信暂时无法打开，请重试'))});
  },fail:()=>reject(new Error('文件下载失败，请检查网络后重试'))});
 });
 // #endif
}
