### ListenAndServe Handler

While convenient for quick scripts, relying on `http.DefaultServeMux` is a notable security and architectural risk for production applications. Fix:

The Production Alternative: `http.NewServeMux()`

To write secure web servers, completely bypass the default global router. Instead, explicitly create a **locally scoped** router that you fully control
