# codeDispatch
 go编写的生产环境代码发布工具，适用于svn环境，原理是将代码从svn仓库checkout出来，执行指定的命令（scripts），然后利用rsync发布到指定服务器

 # 使用前提
 - svn环境
 - rsync在所有服务器都需要安装
 - 生产服务器（dest_hosts）需要支持无密码密钥访问，自行配置好
 - 配置文件提前配置好
# 功能特性
- 支持多服务器多项目同时上线
- 支持版本回退：-r命令
- 支持忽略排除文件或文件夹
- 支持发布到指定服务器： -t命令
- 利用go1.16 embed特性可以把配置文件打包到程序
- 支持发布钱执行额外命令：配置文件中scripts

 # 使用方法
 ```
 codeDispatch -p vue-web,go-api -i 
 ```
 > 首次使用需要加上-i命令，之后就可以不用添加了
 ---
 > 版本回退: -r命令
```
codeDispatch -p vue-web -r
```

---
> 如果go源码是一个目录一个项目，可以支持指定编译
```
codeDispatch -p go-source -g myGoProgram
```
---
> 帮助说明
```
codeDispatch -h
```

# 配置文件说明
```
{
  "NOTICE": "programs里面的program_path必须以/结尾",
  "programs": [
    {
      "program_name": "vue-web", // 项目名称
      "program_path": "/www/vue-web/", // 项目svn本地保存路径
      "dest_path": "/www/vue-web", // 上传到生产服务器路径
      "scripts": [ //上传之前需要执行的命令
        "npm run build"
      ],
      "ignore_files": [ // 忽略上传文件
        "node_modules",
        "public/src"
      ]
    },
    {
      "program_name": "go-api",
      "program_path": "/www/go-api/",
      "dest_path": "/webroot/go-api",
      "scripts": [],
      "go_exec_path": "go", // go类型项目专有，不为空表示为go项目
      "build_source_prefix": "", // program_path目录下一个个文件夹是一个单独项目时目录前缀
      "ignore_files": [
        "test.json",
        "*.go"
      ]
    }
  ],
  "dest_hosts": [ // 生产环境地址配置
    {
      "username": "www",
      "alias": "my-server-name",
      "host": "127.10.0.1",
      "port": 22,
      "key_file": "/www/my-ssh-key" // ssh密钥
    }
  ],
  "ignore_files": [ // 全局忽略文件配置
    ".svn"
  ],
  "save_path": "/www/log/code_dispatch" // 日志路劲，命令只输出概要信息，详细输出查看日志
}
```