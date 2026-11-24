# The Console view

The dashboard has three views, picked in the top bar: **Cards**, **Grid** and **Console**. The Console is made for work, not for watching: no tiles, only a list of servers and the session of the tab you are in. Terminals, RDP and the Assistant of many servers sit side by side in tabs, with no dozen browser tabs. The Console is not shown on a phone.

![The Console view](../screenshots/en/console.png)

## The side list

- **The mode switch** (one row of three buttons, each with an icon and the number of servers that have the mode): **Terminal**, **RDP** and **Assistant**. The mode decides what a click on a server opens. The list shows only the servers that have the mode: RDP and the Assistant for Windows, the Assistant for Linux hosts with a graphical console (see [the Assistant](windows.md) and [modules](modules.md)).
- **Servers** by group (a group folds), each with a status dot, the name and a badge LIN or WIN; an offline server is grey.
- **Filters** at the bottom of the list (a panel that slides up; a number shows how many are on): **OS** (all, Linux, Windows) and **Status** (all, online, offline). They apply to the list and to the numbers on the mode buttons and are remembered in the application.
- **Windows · N** at the bottom: the list of the floating windows (terminals, Files, Dispatcher). A click folds or unfolds a window, the cross closes it.

## Tabs and sessions

- A click on a server opens its session in the chosen mode, or brings the open one to the front (one tab per server and mode). The tabs are on top; when they do not fit, the buttons "<" and ">" scroll them.
- A right click on a server gives **Files** (a floating window) and **Open the server page**.
- Every session bar has the **Files** and **Dispatcher** buttons: the first opens the file manager in a floating window, the second the processes and the services (on Windows and on Linux).
- The sessions belong to the Console and live as long as the page is open: when you switch to another view they keep running in the background; a hidden Assistant is paused to save the traffic.
- The tabs, the mode, the folded groups and the Files windows are remembered in the application, so a reload brings the work back.

## Rights

What a person sees in the Console follows the same rights as the server pages: an operator gets the modules the administrator gave (`terminal`, `desktop` for RDP, `assistant`, `files`, `processes`, `services`), an auditor sees nothing to type into. Every opening of an RDP or Assistant session is written to the audit log.
