export namespace config {
	
	export class ManagedProcess {
	    ID: string;
	    Name: string;
	    Command: string;
	    Args: string;
	    WorkDir: string;
	    AutoStart: boolean;
	    Enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ManagedProcess(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Command = source["Command"];
	        this.Args = source["Args"];
	        this.WorkDir = source["WorkDir"];
	        this.AutoStart = source["AutoStart"];
	        this.Enabled = source["Enabled"];
	    }
	}
	export class PortForward {
	    ID: string;
	    ServerID: string;
	    Name: string;
	    Type: string;
	    LocalAddr: string;
	    LocalPort: number;
	    RemoteHost: string;
	    RemotePort: number;
	    AutoStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortForward(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ServerID = source["ServerID"];
	        this.Name = source["Name"];
	        this.Type = source["Type"];
	        this.LocalAddr = source["LocalAddr"];
	        this.LocalPort = source["LocalPort"];
	        this.RemoteHost = source["RemoteHost"];
	        this.RemotePort = source["RemotePort"];
	        this.AutoStart = source["AutoStart"];
	    }
	}
	export class Server {
	    ID: string;
	    Name: string;
	    Host: string;
	    Port: number;
	    User: string;
	    AuthType: string;
	    Password: string;
	    KeyPath: string;
	    KeyPassphrase: string;
	    HostKeyPolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Host = source["Host"];
	        this.Port = source["Port"];
	        this.User = source["User"];
	        this.AuthType = source["AuthType"];
	        this.Password = source["Password"];
	        this.KeyPath = source["KeyPath"];
	        this.KeyPassphrase = source["KeyPassphrase"];
	        this.HostKeyPolicy = source["HostKeyPolicy"];
	    }
	}
	export class ServerUI {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    authType: string;
	    keyPath: string;
	    hostKeyPolicy: string;
	    hasPassword: boolean;
	    hasPassphrase: boolean;
	    password?: string;
	    keyPassphrase?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerUI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.authType = source["authType"];
	        this.keyPath = source["keyPath"];
	        this.hostKeyPolicy = source["hostKeyPolicy"];
	        this.hasPassword = source["hasPassword"];
	        this.hasPassphrase = source["hasPassphrase"];
	        this.password = source["password"];
	        this.keyPassphrase = source["keyPassphrase"];
	    }
	}
	export class SyncMapping {
	    ID: string;
	    ServerID: string;
	    Name: string;
	    LocalPath: string;
	    RemotePath: string;
	    Excludes: string[];
	    UseGitignore: boolean;
	    DeleteExtra: boolean;
	    Backend: string;
	    CompareMode: string;
	    FileMode: number;
	    DirMode: number;
	    AutoSync: boolean;
	    DebounceMs: number;
	    Enabled: boolean;
	    // Go type: time
	    LastSyncAt: any;
	    LastSyncResult: string;
	    LastSyncError: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ServerID = source["ServerID"];
	        this.Name = source["Name"];
	        this.LocalPath = source["LocalPath"];
	        this.RemotePath = source["RemotePath"];
	        this.Excludes = source["Excludes"];
	        this.UseGitignore = source["UseGitignore"];
	        this.DeleteExtra = source["DeleteExtra"];
	        this.Backend = source["Backend"];
	        this.CompareMode = source["CompareMode"];
	        this.FileMode = source["FileMode"];
	        this.DirMode = source["DirMode"];
	        this.AutoSync = source["AutoSync"];
	        this.DebounceMs = source["DebounceMs"];
	        this.Enabled = source["Enabled"];
	        this.LastSyncAt = this.convertValues(source["LastSyncAt"], null);
	        this.LastSyncResult = source["LastSyncResult"];
	        this.LastSyncError = source["LastSyncError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace forward {
	
	export class Status {
	    Running: boolean;
	    ActiveConns: number;
	    Err: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Running = source["Running"];
	        this.ActiveConns = source["ActiveConns"];
	        this.Err = source["Err"];
	    }
	}

}

export namespace procman {
	
	export class Status {
	    Running: boolean;
	    PID: number;
	    UptimeSec: number;
	    Err: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Running = source["Running"];
	        this.PID = source["PID"];
	        this.UptimeSec = source["UptimeSec"];
	        this.Err = source["Err"];
	    }
	}

}

