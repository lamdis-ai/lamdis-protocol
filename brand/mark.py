"""The Lamdis mark: an L of isometric wireframe cubes (four stacked, one foot).
Prints the SVG path. Every surface that shows the mark is generated from this."""
import sys
SMALL = "--small" in sys.argv   # 32px and under: joints closed, heavier line
W, H, V, G = 4, 2.4, 5.3, (0 if SMALL else 1.0)   # half width, half rhombus height, vertical edge, gap between the top cubes
def cube(cx, ty):
    T=(cx,ty); R=(cx+W,ty+H); F=(cx,ty+2*H); L=(cx-W,ty+H)
    d=lambda p:(p[0],p[1]+V)
    return dict(T=T,R=R,F=F,L=L,Rb=d(R),Fb=d(F),Lb=d(L))
segs=set()
def add(a,b): segs.add(tuple(sorted([tuple(round(x,3) for x in a),tuple(round(x,3) for x in b)])))
cx, base = 6, 2*H+3*V+2*G+0.8   # bottom cube's top vertex y
# The top two joints are open, as on the original: stacked, not fused.
col=[cube(cx, base-k*V-max(0,k-1)*G) for k in range(4)]   # col[0] bottom ... col[3] top
top=col[3]
for a,b in [("T","R"),("R","F"),("F","L"),("L","T")]: add(top[a],top[b])
for k,c in enumerate(col):
    add(c["R"],c["F"]); add(c["F"],c["L"])           # seam / top-front edges
    add(c["L"],c["Lb"]); add(c["F"],c["Fb"])
    if k>0: add(c["R"],c["Rb"])                       # bottom cube's right vertical is hidden by the foot
    add(c["Lb"],c["Fb"])
    if k>=2 and G: add(c["Fb"],c["Rb"])                     # an open joint shows the upper cube's underside
foot=cube(cx+W, base+H)
for a,b in [("T","R"),("R","F"),("F","L"),("L","T")]: add(foot[a],foot[b])
for a,b in [("L","Lb"),("F","Fb"),("R","Rb"),("Lb","Fb"),("Fb","Rb")]: add(foot[a],foot[b])
xs=[p[0] for s in segs for p in s]; ys=[p[1] for s in segs for p in s]
f=lambda n: ("%g"%n)
d=" ".join(f"M{f(a[0])} {f(a[1])}L{f(b[0])} {f(b[1])}" for a,b in sorted(segs))
pad=1
vb=f"{f(min(xs)-pad)} {f(min(ys)-pad)} {f(max(xs)-min(xs)+2*pad)} {f(max(ys)-min(ys)+2*pad)}"
tops=[top, foot]
faces=" ".join("M"+"L".join(f"{f(c[k][0])} {f(c[k][1])}" for k in "TRFL")+"Z" for c in tops)
import sys
if "--svg" in sys.argv:
    mono=f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" fill="none" stroke="currentColor" stroke-width="0.55" stroke-linecap="round" stroke-linejoin="round"><path d="{d}"/></svg>'
    color=(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" fill="none" stroke-linecap="round" stroke-linejoin="round">'
           f'<path d="{faces}" fill="#FFB22E"/><path d="{d}" stroke="#1C1A17" stroke-width="0.55"/></svg>')
    sw = "1" if SMALL else "0.55"
    mono, color = mono.replace('"0.55"', f'"{sw}"'), color.replace('"0.55"', f'"{sw}"')
    tag = "-small" if SMALL else ""
    open(f"lamdis-mark{tag}.svg","w").write(mono+"\n"); open(f"lamdis-mark-color{tag}.svg","w").write(color+"\n")
print(vb); print(d); print(faces)
