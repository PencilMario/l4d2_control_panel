#!/usr/bin/env python3
import collections, glob, json, os, pty, select, shlex, shutil, socket, subprocess, sys, tempfile, threading, time

SOCKET = '/tmp/l4d2-supervisor.sock'
STATUS = '/tmp/l4d2-supervisor.json'
GAME = '/opt/l4d2/game'

class Supervisor:
    def __init__(self, argv, socket_path=SOCKET, status_path=STATUS, ring_bytes=1024*1024):
        self.argv, self.socket_path, self.status_path = argv, socket_path, status_path
        self.ring, self.ring_size, self.ring_limit = collections.deque(), 0, ring_bytes
        self.clients, self.lock, self.stopping = set(), threading.Lock(), False
        self.pid, self.fd, self.started_at, self.exit_code = 0, -1, 0, None

    def _status(self):
        return {'pid': self.pid, 'running': self.exit_code is None, 'uptime': max(0, int(time.time()-self.started_at)), 'last_exit_code': self.exit_code}

    def _write_status(self):
        temporary = self.status_path + '.tmp'
        with open(temporary, 'w', encoding='utf-8') as handle: json.dump(self._status(), handle)
        os.replace(temporary, self.status_path)

    def _remember(self, data):
        with self.lock:
            self.ring.append(data); self.ring_size += len(data)
            while self.ring_size > self.ring_limit and self.ring:
                self.ring_size -= len(self.ring.popleft())
            dead=[]
            for client in self.clients:
                try: client.sendall(data)
                except OSError: dead.append(client)
            for client in dead: self.clients.discard(client)

    def _read_pty(self):
        while True:
            try: data=os.read(self.fd, 16384)
            except OSError: break
            if not data: break
            self._remember(data)

    def _read_client(self, client):
        try:
            while True:
                data=client.recv(65536)
                if not data: break
                os.write(self.fd, data)
        except OSError: pass
        finally:
            with self.lock: self.clients.discard(client)
            try: client.close()
            except OSError: pass

    def _accept(self, server):
        while self.exit_code is None:
            try: client,_=server.accept()
            except socket.timeout: continue
            try:
                command=b''
                while not command.endswith(b'\n') and len(command)<64:
                    part=client.recv(1)
                    if not part: break
                    command+=part
                command=command.decode('ascii','strict').strip()
            except (OSError,UnicodeError): client.close();continue
            if command == 'status': client.sendall(json.dumps(self._status()).encode());client.close()
            elif command == 'stop':
                self.stopping=True;os.write(self.fd,b'quit\n');client.sendall(b'{"accepted":true}');client.close()
            elif command == 'attach':
                with self.lock:
                    for part in self.ring: client.sendall(part)
                    self.clients.add(client)
                threading.Thread(target=self._read_client,args=(client,),daemon=True).start()
            else: client.close()

    def run(self):
        try: os.unlink(self.socket_path)
        except FileNotFoundError: pass
        self.pid,self.fd=pty.fork()
        if self.pid==0: os.execvp(self.argv[0],self.argv)
        self.started_at=time.time();self._write_status()
        server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);server.bind(self.socket_path);os.chmod(self.socket_path,0o600);server.listen(8);server.settimeout(.25)
        threading.Thread(target=self._read_pty,daemon=True).start();threading.Thread(target=self._accept,args=(server,),daemon=True).start()
        _,status=os.waitpid(self.pid,0);self.exit_code=os.waitstatus_to_exitcode(status);self._write_status();server.close()
        with self.lock:
            for client in self.clients:
                try: client.close()
                except OSError: pass
            self.clients.clear()
        return self.exit_code

def request(command):
    client=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);client.connect(SOCKET);client.sendall((command+'\n').encode())
    if command=='attach':
        def stdin_copy():
            try:
                while True:
                    data=os.read(sys.stdin.fileno(),65536)
                    if not data: break
                    client.sendall(data)
            except OSError: pass
        threading.Thread(target=stdin_copy,daemon=True).start()
    while True:
        data=client.recv(65536)
        if not data: break
        os.write(sys.stdout.fileno(),data)

def prepare_content():
    game=os.path.join(GAME,'left4dead2');addons=os.path.join(game,'addons');os.makedirs(addons,exist_ok=True)
    for source in glob.glob('/opt/l4d2/shared-vpk/*.vpk'):
        target=os.path.join(addons,os.path.basename(source))
        if not os.path.exists(os.path.join('/opt/l4d2/private','addons',os.path.basename(source))):
            try:
                if os.path.lexists(target): os.unlink(target)
                os.symlink(source,target)
            except OSError: pass
    private='/opt/l4d2/private'
    if os.path.isdir(private): shutil.copytree(private,game,dirs_exist_ok=True,symlinks=False)

def steamcmd_install_command():
    steamcmd='/home/steam/steamcmd/steamcmd.sh'
    username=os.getenv('STEAM_USERNAME','anonymous');password=os.getenv('STEAM_PASSWORD','')
    login=['+login',username] if username=='anonymous' else ['+login',username,password]
    if username=='anonymous':
        return [steamcmd,'+@sSteamCmdForcePlatformType','windows','+force_install_dir',GAME,*login,'+app_update','222860','+@sSteamCmdForcePlatformType','linux','+app_update','222860','validate','+quit']
    return [steamcmd,'+@sSteamCmdForcePlatformType','linux','+force_install_dir',GAME,*login,'+app_info_update','1','+app_update','222860','validate','+quit']

def ensure_game():
    if os.path.isfile(os.path.join(GAME,'srcds_run')): return
    subprocess.run(steamcmd_install_command(),check=True)

def extra_args():
    raw=os.getenv('SRCDS_EXTRA_ARGS_JSON','').strip()
    if raw:
        value=json.loads(raw)
        if not isinstance(value,list) or not all(isinstance(item,str) for item in value): raise ValueError('SRCDS_EXTRA_ARGS_JSON must be a string array')
        return value
    return shlex.split(os.getenv('SRCDS_EXTRA_ARGS',''))

def srcds_command():
    args=['./srcds_run','-game','left4dead2','-console','-port',os.getenv('SRCDS_PORT','27015'),'-tickrate',os.getenv('SRCDS_TICKRATE','100'),'+map',os.getenv('SRCDS_MAP','c2m1_highway'),'+mp_gamemode',os.getenv('SRCDS_MODE','coop'),'-maxplayers',os.getenv('SRCDS_MAXPLAYERS','8')]
    tv_port=os.getenv('SRCDS_TV_PORT','0').strip()
    if tv_port and tv_port != '0': args.extend(['+tv_enable','1','+tv_port',tv_port])
    args.extend(extra_args());return args

def selftest():
    with tempfile.TemporaryDirectory() as root:
        path=os.path.join(root,'socket');status=os.path.join(root,'status.json');supervisor=Supervisor(['/bin/cat'],path,status,1024)
        thread=threading.Thread(target=supervisor.run,daemon=True);thread.start()
        deadline=time.time()+3
        while not os.path.exists(path) and time.time()<deadline: time.sleep(.01)
        client=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);client.settimeout(2);client.connect(path);client.sendall(b'attach\n');client.sendall(b'hello-pty\n');received=b''
        while b'hello-pty' not in received:
            part=client.recv(4096)
            if not part: raise RuntimeError('attach closed before PTY output')
            received+=part
        client.close();os.kill(supervisor.pid,15);thread.join(2)
        if b'hello-pty' not in received or not os.path.isfile(status): raise RuntimeError('PTY attach self-test failed')

if __name__=='__main__':
    command=sys.argv[1] if len(sys.argv)>1 else ''
    if command=='run':
        ensure_game();prepare_content();failures=[]
        while True:
            supervisor=Supervisor(srcds_command());code=supervisor.run()
            if supervisor.stopping: sys.exit(code)
            now=time.time();failures=[stamp for stamp in failures if now-stamp<300];failures.append(now)
            if len(failures)>int(os.getenv('SRCDS_RESTART_LIMIT','3')): sys.exit(code or 1)
            time.sleep(min(len(failures)*2,10))
    elif command in ('attach','stop','status'): request(command)
    elif command=='selftest': selftest()
    else: sys.exit(64)
