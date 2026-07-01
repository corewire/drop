from pathlib import Path
import numpy as np, pandas as pd, math, zipfile, shutil, os
import matplotlib.pyplot as plt

OUT=Path('/mnt/data/discovery_strategy_fast_20runs')
if OUT.exists(): shutil.rmtree(OUT)
(OUT/'summary').mkdir(parents=True)
(OUT/'figures').mkdir(parents=True)

N_JOBS=25000
N_IMAGES=30
N_NODES=100
DEV_START=9*3600
DEV_END=17*3600
DAY=24*3600
PREWARM_AT=0  # isolate ranking quality: selected images are available after rotation before workload starts
TOPK=10


def hourly_weights():
    w=np.array([0.20,0.16,0.13,0.12,0.12,0.16,0.28,0.45,0.75,1.45,1.55,1.50,1.30,1.25,1.28,1.38,1.45,1.05,0.85,0.70,0.52,0.40,0.30,0.23],float)
    return w/w.sum()


def generate_day(seed, with_batches=True):
    rng=np.random.default_rng(seed)
    ranks=np.arange(1,N_IMAGES+1)
    weights=1/np.power(ranks,0.78)
    weights=weights/weights.sum()
    # make a realistic long tail shuffle for project images but keep common core stable
    tail=weights[10:].copy(); rng.shuffle(tail); weights[10:]=tail
    size_mb=np.clip(rng.lognormal(5.25,0.55,N_IMAGES),70,1200)
    layers=rng.integers(6,24,N_IMAGES)
    p50=np.clip(12 + size_mb/rng.uniform(6.5,12.0,N_IMAGES) + layers*rng.uniform(0.45,1.25,N_IMAGES),18,180)
    # base jobs minus optional scheduled batches
    batch_jobs=[]
    if with_batches:
        # 2 recurring-ish scheduled windows, images not necessarily top globally.
        batch_specs=[]
        # one high-volume validation batch around 01:00 using one image
        img1=int(rng.integers(5,N_IMAGES))
        batch_specs.append((3600, int(rng.integers(450,750)), img1, 35*60))
        # sometimes a second smaller batch around 02:30 using another image
        if rng.random()<0.75:
            img2=int(rng.integers(0,N_IMAGES))
            batch_specs.append((9000, int(rng.integers(200,450)), img2, 25*60))
        for center,count,img,duration in batch_specs:
            # spread across +/- duration/2 with clustering
            offsets=np.clip(rng.normal(0,duration/5,count),-duration/2,duration/2)
            times=np.clip(center+offsets,0,DAY-1)
            for t in times:
                batch_jobs.append((float(t), img))
    base_count=N_JOBS-len(batch_jobs)
    hcounts=rng.multinomial(base_count,hourly_weights())
    times=[]
    for hour,count in enumerate(hcounts):
        if 9<=hour<=17:
            offsets=rng.beta(2.0,2.0,count)*3600
        else:
            offsets=rng.uniform(0,3600,count)
        times.extend((hour*3600+offsets).tolist())
    times=np.array(times)
    imgs=rng.choice(np.arange(N_IMAGES),size=base_count,p=weights)
    if batch_jobs:
        btimes=np.array([x[0] for x in batch_jobs])
        bimgs=np.array([x[1] for x in batch_jobs])
        times=np.concatenate([times,btimes])
        imgs=np.concatenate([imgs,bimgs])
    order=np.argsort(times)
    times=times[order]
    imgs=imgs[order]
    nodes=rng.integers(0,N_NODES,len(times))
    stages=rng.choice(np.arange(6),size=len(times),p=[0.08,0.20,0.45,0.16,0.08,0.03])
    stage_means=np.array([35,210,150,360,120,75],float)
    runtime=np.maximum(10,rng.lognormal(np.log(stage_means[stages]),0.45))
    finish=times+runtime
    return dict(times=times, imgs=imgs, nodes=nodes, runtime=runtime, finish=finish, p50=p50, weights=weights)


def dev_weight_seconds(t):
    h=t/3600.0
    if 9<=h<17: return 1.0
    if 7<=h<9 or 17<=h<20: return 0.3
    return 0.0
v_dev_weight=np.vectorize(dev_weight_seconds)


def rankings(day, recent_window=2*3600):
    t=day['times']; img=day['imgs']; finish=day['finish']
    # Discovery from whole historical day.
    S=np.bincount(img,minlength=N_IMAGES).astype(float)
    W=np.bincount(img,weights=v_dev_weight(t),minlength=N_IMAGES).astype(float)
    recent_mask=t>=DAY-recent_window
    R=np.bincount(img[recent_mask],minlength=N_IMAGES).astype(float)
    # peak active concurrency per image via sweep line
    C=np.zeros(N_IMAGES,float)
    for i in range(N_IMAGES):
        idx=np.where(img==i)[0]
        events=[]
        for j in idx:
            events.append((t[j],1)); events.append((finish[j],-1))
        active=0; peak=0
        for _,d in sorted(events, key=lambda x:(x[0],x[1])):
            active+=d; peak=max(peak,active)
        C[i]=peak
    def order(score):
        return list(np.argsort(-score))
    def norm(x):
        if x.max()==x.min(): return np.ones_like(x,dtype=float)
        return (x-x.min())/(x.max()-x.min())
    H=0.5*norm(S)+0.5*norm(C)
    return {'count':order(S),'dev_weighted_count':order(W),'recent_count':order(R),'peak_concurrency':order(C),'hybrid_count_concurrency_a0.5':order(H)}, {'count':S,'dev_weighted_count':W,'recent_count':R,'peak_concurrency':C,'hybrid_count_concurrency_a0.5':H}


def replay(day, prewarm_set=()):
    t=day['times']; img=day['imgs']; node=day['nodes']; p50=day['p50']
    availability={}
    waits=np.zeros(len(t),float)
    cold=np.zeros(len(t),bool)
    for j in range(len(t)):
        key=(int(node[j]), int(img[j]))
        S=float(t[j])
        if key not in availability:
            if int(img[j]) in prewarm_set and S>=PREWARM_AT:
                availability[key]=PREWARM_AT
            else:
                # per-job variation but deterministic-ish by image/job not needed
                T=S+float(p50[img[j]])
                availability[key]=T
                waits[j]=float(p50[img[j]])
                cold[j]=True
        else:
            T=availability[key]
            if S<T:
                waits[j]=T-S; cold[j]=True
    return waits,cold


def metrics(day, waits, cold):
    t=day['times']
    dev=(t>=DEV_START)&(t<DEV_END)
    return dict(
        affected_jobs_total=int(cold.sum()),
        affected_job_minutes_total=float(waits.sum()/60),
        affected_jobs_developer_window=int(cold[dev].sum()),
        affected_job_minutes_developer_window=float(waits[dev].sum()/60),
        p95_wait_seconds_developer_window=float(np.quantile(waits[dev],0.95)) if dev.any() else 0,
        p99_wait_seconds_developer_window=float(np.quantile(waits[dev],0.99)) if dev.any() else 0,
    )

rows=[]
rank_rows=[]
for run in range(20):
    seed=20260621+run
    hist=generate_day(seed, True)
    evalday=generate_day(seed+10000, True)
    rnk,scores=rankings(hist)
    base_w,base_c=replay(evalday,set())
    base=metrics(evalday,base_w,base_c)
    # oracle based on evaluation baseline, dev window impact upper bound
    impact=np.zeros(N_IMAGES)
    dev=(evalday['times']>=DEV_START)&(evalday['times']<DEV_END)
    for i in range(N_IMAGES):
        impact[i]=base_w[(evalday['imgs']==i)&dev].sum()/60
    rnk['oracle_impact_upper_bound']=list(np.argsort(-impact))
    for strat,order in rnk.items():
        for rank,imgid in enumerate(order,1):
            rank_rows.append({'run':run+1,'seed':seed,'strategy':strat,'rank':rank,'image_id':f'img-{imgid+1:02d}'})
        selected=set(order[:TOPK])
        w,c=replay(evalday,selected)
        m=metrics(evalday,w,c)
        row={'run':run+1,'seed':seed,'strategy':strat,'top_k':TOPK}
        row.update(m)
        row['minutes_avoided_total']=base['affected_job_minutes_total']-m['affected_job_minutes_total']
        row['percent_minutes_saved_total']=100*row['minutes_avoided_total']/base['affected_job_minutes_total']
        row['minutes_avoided_developer_window']=base['affected_job_minutes_developer_window']-m['affected_job_minutes_developer_window']
        row['percent_minutes_saved_developer_window']=100*row['minutes_avoided_developer_window']/base['affected_job_minutes_developer_window']
        row['affected_jobs_avoided_developer_window']=base['affected_jobs_developer_window']-m['affected_jobs_developer_window']
        rows.append(row)
    print('run',run+1,'baseline total min',round(base['affected_job_minutes_total'],1),'dev',round(base['affected_job_minutes_developer_window'],1))

all_df=pd.DataFrame(rows)
all_df.to_csv(OUT/'summary'/'strategy_comparison_all_runs.csv',index=False)
pd.DataFrame(rank_rows).to_csv(OUT/'summary'/'discovery_rankings_all_runs.csv',index=False)
agg=all_df.groupby(['strategy','top_k']).agg(
    mean_affected_job_minutes_total=('affected_job_minutes_total','mean'),
    std_affected_job_minutes_total=('affected_job_minutes_total','std'),
    mean_percent_saved_total=('percent_minutes_saved_total','mean'),
    std_percent_saved_total=('percent_minutes_saved_total','std'),
    mean_affected_job_minutes_developer_window=('affected_job_minutes_developer_window','mean'),
    mean_percent_saved_developer_window=('percent_minutes_saved_developer_window','mean'),
    std_percent_saved_developer_window=('percent_minutes_saved_developer_window','std'),
    mean_affected_jobs_developer_window=('affected_jobs_developer_window','mean'),
    mean_p95_wait_seconds_developer_window=('p95_wait_seconds_developer_window','mean'),
).reset_index().sort_values('mean_percent_saved_developer_window',ascending=False)
agg.to_csv(OUT/'summary'/'strategy_summary_top10.csv',index=False)
print('\nSUMMARY')
print(agg.to_string(index=False))

# plots
plot_df=agg.sort_values('mean_percent_saved_developer_window',ascending=True)
plt.figure(figsize=(8,4.8))
plt.barh(plot_df['strategy'], plot_df['mean_percent_saved_developer_window'])
plt.xlabel('Mean developer-window affected job-minutes saved (%)')
plt.title('Discovery strategies: developer-window savings (top 10)')
plt.tight_layout()
plt.savefig(OUT/'figures'/'strategy_dev_window_savings_top10.png',dpi=180)
plt.close()

plot_df=agg.sort_values('mean_percent_saved_total',ascending=True)
plt.figure(figsize=(8,4.8))
plt.barh(plot_df['strategy'], plot_df['mean_percent_saved_total'])
plt.xlabel('Mean all-day affected job-minutes saved (%)')
plt.title('Discovery strategies: all-day cold exposure savings (top 10)')
plt.tight_layout()
plt.savefig(OUT/'figures'/'strategy_total_savings_top10.png',dpi=180)
plt.close()

# zip results
zip_path=Path('/mnt/data/discovery-strategy-20runs-results.zip')
if zip_path.exists(): zip_path.unlink()
with zipfile.ZipFile(zip_path,'w',zipfile.ZIP_DEFLATED) as z:
    for p in OUT.rglob('*'):
        if p.is_file():
            z.write(p,p.relative_to(OUT.parent))
print('wrote',zip_path)
