
![LOGO](https://qn.static.buting.cc/img/httpshare.png)

# HTTP SHARE
一款极致简约的文件共享软件


## 功能介绍
- 将文件、文件夹通过右键菜单快速显示成二维码，可以使得手机扫码后快速提取文件。
- 同时显示多种网络连接，本地网络可以用，IPv6可以直接访问，如果有公网IP也可以共享给其他人。
- 命令行模式，方便调用

## 使用方法

```shell

hs help  查看帮助指令
hs -i 要共享的文件或者文件夹路径

```

> 具体参数:
> hs [命令] [参数]
>
> 主命令 (直接启动服务):
> hs -i <路径> [-p 端口] [-pass 密码] [-m 模式] [-h 域名]
> -i string    输入文件或目录路径 (必须项或默认当前目录)
> -p string    指定服务端口 (默认 1120)
> -pass string 开启密码保护
> -m string    网络模式优先渲染二维码: lan, ipv6, custom (默认 lan)
> -h string    自定义外网 IP 或域名 (用于 custom 模式)

配置命令 (持久化修改默认设置):
hs config [参数]
hs config -p 8080 -pass 123456 -m ipv6 -h share.com

其他命令:
hs help        显示此帮助信息
hs v        显示版本信息


## Windows上使用菜单

如果要集成到右键菜单中，请运行 install.reg 文件，将程序添加到右键菜单中去。



## 作者
博客地址：https://buting.cc  
给我写信：my@buting.cc


## 截图

![web页面](/img/web.png)
![命令行](/img/cmd.png)



